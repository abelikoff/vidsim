package state

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/badger/v3"
	"github.com/sirupsen/logrus"
)

type matchScore struct {
	Score         float32 // comparison score [0..1]
	FalsePositive bool    // true for false positives
}

type State struct {
	dataDirectory string
	persistent    bool           // should we load and save the state?
	image2frame   map[string]int // video filename -> frame ID
	frame2image   map[int]string // frame ID -> video filename
	nextframeID   int

	matchScores map[[2]int]matchScore // pair of frame IDs (ordered numerically) -> match score information

	mutex  *sync.RWMutex
	db     *badger.DB
	logger *logrus.Logger
}

func MakeState() *State {
	state := new(State)

	state.mutex = new(sync.RWMutex)
	state.image2frame = make(map[string]int)
	state.frame2image = make(map[int]string)
	state.matchScores = make(map[[2]int]matchScore)
	state.nextframeID = 1

	return state
}

func (state *State) Init(stateDirectory string, logger *logrus.Logger) error {
	state.logger = logger

	if stateDirectory == "" {
		dirName, err := os.MkdirTemp("", "vidsim")

		if err != nil {
			state.logger.Fatalf("Failed to create a temporary directory: %s", err)
		}

		state.dataDirectory = dirName
		state.persistent = false
	} else {
		state.dataDirectory = stateDirectory
		state.persistent = true
	}

	if state.dataDirectory == "." {
		return errors.New("don't use the current directory to keep the state")
	}

	if state.persistent {
		var err error
		state.db, err = badger.Open(badger.DefaultOptions(filepath.Join(state.dataDirectory, "db")).WithLogger(nil))

		if err != nil {
			return err
		}

		maxID, err := state.getMaxFrameID()

		if err != nil {
			return err
		}

		state.nextframeID = maxID + 1
		state.logger.Debugf("Next frame ID: %d", maxID)
	}

	return nil
}

func (state *State) Close() {
	state.db.Close()
}

func (state *State) RegisterFile(path string) (int, bool) {
	// state.mutex.Lock()
	// defer state.mutex.Unlock()

	var frameID int
	var found bool

	if state.persistent {
		frameID, found = state.AddFrameIDPersistent(path)
	} else {
		frameID, found = state.image2frame[path]

		if !found {
			frameID = state.nextframeID
			state.nextframeID++
			state.image2frame[path] = frameID
		}
	}

	state.frame2image[frameID] = path
	return frameID, found
}

func (state *State) DeleteFile(path string) {
	// state.mutex.Lock()
	// defer state.mutex.Unlock()

	state.logger.Fatal("DeleteFile called")
	delete(state.image2frame, path)
}

func (state *State) GetframeID(path string) (int, bool) {
	// state.mutex.RLock()
	// defer state.mutex.RUnlock()

	if state.persistent {
		frameID, found := state.getImageFramePersistent(path)

		if found {
			state.frame2image[frameID] = path
		}

		return frameID, found
	}

	frameID, found := state.image2frame[path]
	return frameID, found
}

func (state *State) SetframeID(path string, frameID int) {
	// state.mutex.Lock()
	// defer state.mutex.Unlock()

	if state.persistent {
		state.setFileFrameIDPersistent(path, frameID)
		return
	}

	state.image2frame[path] = frameID
	state.frame2image[frameID] = path
}

func (state *State) GetImageFile(frameID int) (string, bool) {
	// state.mutex.RLock()
	// defer state.mutex.RUnlock()

	path, found := state.frame2image[frameID]
	return path, found
}

func (state *State) GetFrameFileName(frameID int) string {
	return filepath.Join(state.dataDirectory, fmt.Sprintf("frame%06d.jpg", frameID))
}

func (state *State) GetComparisonScore(frameID1 int, frameID2 int) (float32, bool) {
	if state.persistent {
		score, found := state.getComparisonScorePersistent(frameID1, frameID2)
		return score, found
	}

	// make sure frame IDs are ordered

	if frameID1 > frameID2 {
		frameID2, frameID1 = frameID1, frameID2
	}

	key := [2]int{frameID1, frameID2}

	state.mutex.RLock()
	info, found := state.matchScores[key]
	state.mutex.RUnlock()

	if !found {
		return 0, found
	}

	if info.FalsePositive {
		return -info.Score, true
	}

	return info.Score, true
}

func (state *State) SetComparisonScore(frameID1 int, frameID2 int, score float32) {
	if state.persistent {
		state.setComparisonScorePersistent(frameID1, frameID2, score)
		return
	}

	// make sure frame IDs are ordered

	if frameID1 > frameID2 {
		frameID2, frameID1 = frameID1, frameID2
	}

	key := [2]int{frameID1, frameID2}
	state.mutex.Lock()
	state.matchScores[key] = matchScore{Score: score, FalsePositive: false}
	state.mutex.Unlock()
}

func (state *State) UnmatchFrames(frameID1, frameID2 int, falsePositive bool) {
	if !state.persistent {
		state.logger.Error("Unmatching only supported with persistent state")
		return
	}

	state.unmatchFramesPersistent(frameID1, frameID2, falsePositive)
}

func (state *State) DebugDump() {
	state.logger.Debugf("--- scores ------------------\n%v\n", state.matchScores)
}

// ************************** Persistence methods ***********************************

func (state *State) getMaxFrameID() (int, error) {
	if !state.persistent {
		return 0, errors.New("only supported with persistence")
	}

	var maxFrameID int = 0
	prefix := []byte(framePrefix)
	err := state.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Optimize for key-only iteration
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			//key := item.Key()

			// Extract integer from value bytes
			err := item.Value(func(val []byte) error {
				frameID := decodeFrameValue(val)

				if frameID > maxFrameID {
					maxFrameID = frameID
				}

				return nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return maxFrameID, nil
}

func (state *State) AddFrameIDPersistent(path string) (int, bool) {
	// state.mutex.Lock()
	// defer state.mutex.Unlock()

	frameID := -1
	found := false

	err := state.db.Update(func(txn *badger.Txn) error {
		key := encodeFrameKey(path)
		item, err := txn.Get(key)

		if err == nil { // record with a given key found
			err = item.Value(func(val []byte) error {
				frameID = decodeFrameValue(val)
				return nil
			})

			if err != nil {
				return err
			}

			found = true
			return nil
		}

		// record not found - create it

		frameID = state.nextframeID
		state.nextframeID++

		if err = txn.Set(key, encodeFrameValue(frameID)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		state.logger.Errorf("AddFrameIDPersistent('%s'): %s", path, err)
	}

	return frameID, found
}

func (state *State) GetFileFrameIDPersistent(path string) (int, bool) {
	// state.mutex.Lock()
	// defer state.mutex.Unlock()

	frameID := -1
	found := false

	err := state.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(path))

		if err == nil { // record with a given key found
			err = item.Value(func(val []byte) error {
				frameID = decodeFrameValue(val)
				return nil
			})

			if err != nil {
				return err
			}

			found = true
			return nil
		}

		return nil
	})

	if err != nil {
		state.logger.Errorf("GetFileFrameIDPersistent('%s'): %s", path, err)
		return 0, false
	}

	return frameID, found
}

func (state *State) getImageFramePersistent(path string) (int, bool) {
	var valCopy []byte

	err := state.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(encodeFrameKey(path))

		if err != nil {
			return err // Key not found or other error
		}

		valCopy, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		state.logger.Errorf("getImageFramePersistent('%s'): %s", path, err)
		return 0, false
	}

	return decodeFrameValue(valCopy), true
}

func (state *State) getComparisonScorePersistent(frameID1, frameID2 int) (float32, bool) {
	var score float32
	var falsePositive bool

	err := state.db.View(func(txn *badger.Txn) error {
		key := encodeScoreKey(frameID1, frameID2)
		item, err := txn.Get(key)

		if err != nil {
			return err // Key not found or other error
		}

		err = item.Value(func(val []byte) error {
			score, falsePositive = decodeScoreData(val)
			return nil
		})

		return err
	})

	if err != nil {
		if err != badger.ErrKeyNotFound {
			state.logger.Errorf("GetComparisonScorePersistent(%d, %d): %s",
				frameID1, frameID2, err)
		}

		return 0, false
	}

	if falsePositive {
		score = -score
	}

	return score, true
}

func (state *State) setFileFrameIDPersistent(path string, frameID int) {
	err := state.db.Update(func(txn *badger.Txn) error {
		return txn.Set(encodeFrameKey(path), []byte{byte(frameID)})
	})

	if err != nil {
		state.logger.Errorf("setFileFrameIDPersistent('%s'): %s", path, err)
	}
}

func (state *State) setComparisonScorePersistent(frameID1, frameID2 int, score float32) {
	err := state.db.Update(func(txn *badger.Txn) error {
		val, err := encodeScoreData(score, false)

		if err != nil {
			return err
		}

		return txn.Set(encodeScoreKey(frameID1, frameID2), val)
	})

	if err != nil {
		state.logger.Errorf("setComparisonScorePersistent(%d, %d): %s",
			frameID1, frameID2, err)
	}
}

func (state *State) unmatchFramesPersistent(frameID1, frameID2 int, falsePositive bool) {
	err := state.db.Update(func(txn *badger.Txn) error {
		key := encodeScoreKey(frameID1, frameID2)
		item, err := txn.Get(key)

		if err != nil {
			return err
		}

		err = item.Value(func(val []byte) error {
			val[4] = boolToByte(falsePositive) // Update only the IsValid byte
			return txn.Set(key, val)
		})
		return err
	})

	if err != nil {
		state.logger.Errorf("setScoreAsFalsePositivePersistent(%d, %d): %s",
			frameID1, frameID2, err)
	}
}

func (state *State) CompactDataStore() error {
	if !state.persistent {
		return errors.New("only supported with persistence")
	}

	prefix := []byte(framePrefix)

	// Step 1 - make sure we are in the right directory. Filenames are stored as relative paths so running
	// from a wrong place might result in "not files exist anymore" situation, effectively wiping out the state.

	const minViableFraction = 0.4 // at least 40% of files should exist in order to start deleting the entries
	numFrameEntries := 0
	numExistingFiles := 0

	err := state.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // only need keys
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			filename := decodeFrameKey(item.Key())
			numFrameEntries++

			if _, err := os.Stat(filename); !os.IsNotExist(err) {
				numExistingFiles++
			}
		}

		return nil
	})

	if err != nil {
		state.logger.Errorf("Error during frames counting: %s", err)
	}

	if numFrameEntries == 0 {
		state.logger.Debug("No entries to compact")
		return nil
	}

	if float32(numExistingFiles)/float32(numFrameEntries) < minViableFraction {
		state.logger.Errorf("Of %d entries in the DB, only %d files are present -- aborting compaction", numFrameEntries, numExistingFiles)
		return nil
	}

	// Step 2 - delete frame mapping entries that correspond to files that no longer exist.

	validFrames := make(map[int]bool)    // collect all valid frameIDs for Step 3
	validImages := make(map[string]bool) // collect all valid image filenames for Step 4
	numFrameEntriesDeleted := 0
	numScoreEntries := 0
	numScoreEntriesDeleted := 0

	err = state.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			filename := decodeFrameKey(key)

			// delete frame entries for non-existent files

			if _, err := os.Stat(filename); os.IsNotExist(err) {
				state.logger.Debugf("Deleting frame record for '%s'", filename)
				err := txn.Delete(key)

				if err != nil {
					state.logger.Errorf("Failed to delete frame record for '%s': %s", filename, err)
					continue
				}

				numFrameEntriesDeleted++
			} else {
				value, err := item.ValueCopy(nil)

				if err != nil {
					state.logger.Errorf("Failed to extract frameID for '%s': %s", filename, err)
					continue
				}

				frameID := decodeFrameValue(value)
				validFrames[frameID] = true
				validImages[state.GetFrameFileName(frameID)] = true
			}
		}
		return nil
	})

	if err != nil {
		state.logger.Errorf("Error during frames compaction: %s", err)
	}

	// Step 3 - delete comparison (score) records that reference the files that no longer exist.

	err = state.db.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		const maxBatchSize = 1000
		batchSize := 0
		wb := state.db.NewWriteBatch()

		for it.Seek(scorePrefix); it.ValidForPrefix(scorePrefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			numScoreEntries++
			frameID1, frameID2 := decodeScoreKey(key)

			if !validFrames[frameID1] || !validFrames[frameID2] {
				state.logger.Debugf("Deleting score record for frame IDs %d, %d", frameID1, frameID2)
				// err = txn.Delete(key)
				err = wb.Delete(key)

				if err != nil {
					state.logger.Errorf("Failed to delete score record for '%d/%d': %v", frameID1, frameID2, err)
				}

				numScoreEntriesDeleted++
				batchSize++

				if batchSize >= maxBatchSize {
					if err = wb.Flush(); err != nil {
						state.logger.Errorf("Failed to flush batch: %v", err)
					}

					wb = state.db.NewWriteBatch()
					batchSize = 0
				}
			}
		}

		wb.Cancel()
		return nil
	})

	if err != nil {
		state.logger.Errorf("Error during scores compaction: %v", err)
	}

	err = state.db.RunValueLogGC(0.5) // GC the log

	if err != nil && err != badger.ErrNoRewrite {
		state.logger.Errorf("Error during log garbage compaction: %v", err)
	}

	// Step 4 - clean up stale image files

	files, err := filepath.Glob(filepath.Join(state.dataDirectory, "*.jpg"))
	numImageFilesProcessed := 0
	numImageFilesDeleted := 0

	if err != nil {
		state.logger.Errorf("Failed to glob image files: %v", err)
	} else {
		for _, imageFile := range files {
			numImageFilesProcessed++

			if !validImages[imageFile] {
				state.logger.Debugf("Deleting stale image file '%s'", imageFile)
				os.Remove(imageFile)
				numImageFilesDeleted++
			}
		}
	}

	frameDeletePercentage := 0
	scoreDeletePercentage := 0
	fileDeletePercentage := 0

	if numFrameEntries > 0 {
		frameDeletePercentage = int(float64(numFrameEntriesDeleted) / float64(numFrameEntries) * 100.0)
	}

	if numScoreEntries > 0 {
		scoreDeletePercentage = int(float64(numScoreEntriesDeleted) / float64(numScoreEntries) * 100.0)
	}

	if numImageFilesProcessed > 0 {
		fileDeletePercentage = int(float64(numImageFilesDeleted) / float64(numImageFilesProcessed) * 100.0)
	}

	fmt.Printf(`
Summary:
* Deleted %d (%d%%) out of %d frame mapping records.
* Deleted %d (%d%%) out of %d comparison score records.
* Deleted %d (%d%%) out of %d frame files.

`, numFrameEntriesDeleted, frameDeletePercentage, numFrameEntries,
		numScoreEntriesDeleted, scoreDeletePercentage, numScoreEntries,
		numImageFilesDeleted, fileDeletePercentage, numImageFilesProcessed)

	return nil
}

var framePrefix = "f:"
var scorePrefix = []byte("s:")
var prefixKeyLength = -1

func encodeFrameKey(path string) []byte {
	return []byte(framePrefix + path)
}

// Extract the filename from encoded frame key

func decodeFrameKey(encoded []byte) string {
	if prefixKeyLength < 0 {
		prefixKeyLength = len([]byte(framePrefix))
	}

	return string(encoded[prefixKeyLength:])
}

func encodeFrameValue(frameID int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(frameID))
	return key
}

func decodeFrameValue(encoded []byte) int {
	return int(binary.BigEndian.Uint64(encoded))
}

func encodeScoreKey(frameID1, frameID2 int) []byte {
	keyLen := len(scorePrefix) + 2*8 // prefix + 2 * uint64

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	key := make([]byte, keyLen)
	copy(key, scorePrefix)
	offset := len(scorePrefix)
	binary.BigEndian.PutUint64(key[offset:], uint64(frameID1))
	offset += 8
	binary.BigEndian.PutUint64(key[offset:], uint64(frameID2))
	return key
}

func decodeScoreKey(encoded []byte) (int, int) {
	prefixLen := len(scorePrefix)
	frameID1 := int(binary.BigEndian.Uint64(encoded[prefixLen : prefixLen+8]))
	frameID2 := int(binary.BigEndian.Uint64(encoded[prefixLen+8:]))
	return frameID1, frameID2
}

func encodeScoreData(score float32, falsePositive bool) ([]byte, error) {
	b := make([]byte, 5) // 4 bytes (float32) + 1 byte (bool)
	binary.BigEndian.PutUint32(b, math.Float32bits(score))
	b[4] = boolToByte(falsePositive)
	return b, nil
}

func decodeScoreData(encoded []byte) (float32, bool) {
	score := math.Float32frombits(binary.BigEndian.Uint32(encoded[:4]))
	falsePositive := encoded[4] != 0
	return score, falsePositive
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
