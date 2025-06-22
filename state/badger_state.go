// ==========================================================================
// vidsim - a tool to compare and group large sets of images for similarity.
//
// Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
//
// This software is released under the BSD 3-Clause License. See the LICENSE
// file for details.
//
// https://github.com/abelikoff/vidsim
//
// ==========================================================================

package state

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/abelikoff/vidsim/util"
	"github.com/dgraph-io/badger/v3"
	"github.com/sirupsen/logrus"
)

type BadgerBackedState struct {
	dataDirectory   string
	framePrefix     string // prefix used by frame mapping records
	scorePrefix     []byte // prefix used by comparison score records
	nextframeID     int
	prefixKeyLength int

	id2file map[int]string // file ID -> video filename (searching in DB is suboptimal)
	db      *badger.DB
	logger  *logrus.Logger
}

func MakeBadgerBackedState(stateDirectory string, logger *logrus.Logger,
	scorePrefix string) (*BadgerBackedState, error) {
	state := new(BadgerBackedState)
	state.prefixKeyLength = -1
	state.logger = logger
	state.dataDirectory = stateDirectory

	if state.dataDirectory == "." {
		return nil, errors.New("don't use the current directory to keep the state")
	}

	state.framePrefix = "f:"

	if scorePrefix != "" {
		state.scorePrefix = []byte("s:" + scorePrefix + ":")
	} else {
		state.scorePrefix = []byte("s:")
	}

	state.id2file = make(map[int]string)

	var err error
	state.db, err = badger.Open(badger.DefaultOptions(filepath.Join(state.dataDirectory, "db")).WithLogger(nil))

	if err != nil {
		return nil, err
	}

	maxID, err := state.getMaxFrameID()

	if err != nil {
		return nil, err
	}

	state.nextframeID = maxID + 1
	state.logger.Debugf("Next frame ID: %d", state.nextframeID)

	return state, nil
}

func (state *BadgerBackedState) Close() {
	state.id2file = nil
	state.db.Close()
}

func (state *BadgerBackedState) AddFile(path string) (int, bool) {
	frameID := -1
	found := false

	err := state.db.Update(func(txn *badger.Txn) error {
		key := state.encodeFrameKey(path)
		item, err := txn.Get(key)

		if err == nil { // record with a given key found
			err = item.Value(func(val []byte) error {
				frameID = state.decodeFrameValue(val)
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

		if err = txn.Set(key, state.encodeFrameValue(frameID)); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		state.logger.Errorf("AddFile('%s'): %s", path, err)
	}

	state.id2file[frameID] = path
	return frameID, found
}

// Get ID for an already added file
//
// Returns frame ID and true if the file was found, false otherwise

func (state *BadgerBackedState) GetFileID(path string) (int, bool) {
	var valCopy []byte

	err := state.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(state.encodeFrameKey(path))

		if err != nil {
			return err // Key not found or other error
		}

		valCopy, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		state.logger.Debugf("GetFileID('%s'): %s", path, err)
		return 0, false
	}

	return state.decodeFrameValue(valCopy), true
}

// Get video file for ID

func (state *BadgerBackedState) GetVideoFile(id int) (string, bool) {
	file, found := state.id2file[id]
	return file, found
}

// Scan all files in the state and call the callback function for each file.
// The callback should return true to continue scanning, false to stop.
func (state *BadgerBackedState) ScanFiles(callback func(id int, path string) bool) {
	prefix := []byte(state.framePrefix)

	err := state.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			path := state.decodeFrameKey(item.Key())

			var id int
			err := item.Value(func(val []byte) error {
				id = state.decodeFrameValue(val)
				return nil
			})

			if err != nil {
				state.logger.Errorf("Error decoding frame value: %s", err)
				continue
			}

			// Store in id2file map if not already there
			if _, ok := state.id2file[id]; !ok {
				state.id2file[id] = path
			}

			if !callback(id, path) {
				break
			}
		}

		return nil
	})

	if err != nil {
		state.logger.Errorf("Error scanning files: %s", err)
	}
}

// Get name of the image frame file corresponding to the frame ID

func (state *BadgerBackedState) GetFrameFile(frameID int) string {
	return filepath.Join(state.dataDirectory, fmt.Sprintf("frame%06d.jpg", frameID))
}

// Get comparison score for two files
//
// Returns 3 values:
// - the score
// - whether two images are considered a false positive match
// - true if the score was found, false otherwise

func (state *BadgerBackedState) GetComparisonScore(frameID1, frameID2 int) (float32, bool, bool) {
	var score float32
	var falsePositive bool

	err := state.db.View(func(txn *badger.Txn) error {
		key := state.encodeScoreKey(frameID1, frameID2)
		item, err := txn.Get(key)

		if err != nil {
			return err // Key not found or other error
		}

		err = item.Value(func(val []byte) error {
			score, falsePositive = state.decodeScoreData(val)
			return nil
		})

		return err
	})

	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) {
			state.logger.Errorf("GetComparisonScore(%d, %d): %s",
				frameID1, frameID2, err)
		}

		return 0, false, false
	}

	return score, falsePositive, true
}

// Set comparison score for two files

func (state *BadgerBackedState) SetComparisonScore(frameID1, frameID2 int, score float32, isFalsePositive bool) {
	state.logger.Debugf("saving score for %d, %d => %.4f (fp: %v)", frameID1, frameID2, score, isFalsePositive)

	err := state.db.Update(func(txn *badger.Txn) error {
		val, err := state.encodeScoreData(score, isFalsePositive)

		if err != nil {
			return err
		}

		return txn.Set(state.encodeScoreKey(frameID1, frameID2), val)
	})

	if err != nil {
		state.logger.Errorf("setComparisonScore(%d, %d): %s",
			frameID1, frameID2, err)
	}
}

func (state *BadgerBackedState) Compact(stats *util.CompactionStats) error {
	prefix := []byte(state.framePrefix)

	// Step 1 - make sure we are in the right directory. Filenames are stored as relative paths so running
	// from a wrong place might result in "not files exist anymore" situation, effectively wiping out the state.

	const minViableFraction = 0.4 // at least 40% of files should exist in order to start deleting the entries
	numFrameEntries := 0
	numExistingFiles := 0

	state.logger.Debug("STAGE 1: checking against an extinction event")
	err := state.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // only need keys
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			filename := state.decodeFrameKey(item.Key())
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
		state.logger.Errorf("Of %d entries in the DB, only %d files are present -- aborting compaction",
			numFrameEntries, numExistingFiles)
		return nil
	}

	// Step 2 - delete frame mapping entries that correspond to files that no longer exist.

	state.logger.Debug("STAGE 2: removing stale frame records")
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
			filename := state.decodeFrameKey(key)

			// delete frame entries for non-existent files

			if _, err := os.Stat(filename); os.IsNotExist(err) {
				state.logger.Debugf("Deleting frame record for '%s'", filename)
				err = txn.Delete(key)

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

				frameID := state.decodeFrameValue(value)
				validFrames[frameID] = true
				validImages[state.GetFrameFile(frameID)] = true
			}
		}
		return nil
	})

	if err != nil {
		state.logger.Errorf("Error during frames compaction: %s", err)
	}

	// Step 3 - delete comparison (score) records that reference the files that no longer exist.

	state.logger.Debug("STAGE 3: removing stale comparison scores")
	err = state.db.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		const maxBatchSize = 1000
		batchSize := 0
		wb := state.db.NewWriteBatch()

		for it.Seek(state.scorePrefix); it.ValidForPrefix(state.scorePrefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			numScoreEntries++
			frameID1, frameID2 := state.decodeScoreKey(key)

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
					state.logger.Debugf("Flushing work batch (%d)", batchSize)

					if err = wb.Flush(); err != nil {
						state.logger.Errorf("Failed to flush batch: %v", err)
					}

					wb = state.db.NewWriteBatch()
					batchSize = 0
				}
			}
		}

		state.logger.Debug("Flushing final work batch")

		if err = wb.Flush(); err != nil {
			state.logger.Errorf("Failed to flush batch: %v", err)
		}
		// wb.Cancel()
		return nil
	})

	if err != nil {
		state.logger.Errorf("Error during scores compaction: %v", err)
	}

	state.logger.Debug("Garbage collecting")
	gcDiscardRatio := 0.5
	err = state.db.RunValueLogGC(gcDiscardRatio) // GC the log

	if err != nil && !errors.Is(err, badger.ErrNoRewrite) {
		state.logger.Errorf("Error during log garbage compaction: %v", err)
	}

	// Step 4 - clean up stale image files

	state.logger.Debug("STAGE 4: removing stale image files")
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

	stats.FrameFiles.WholeValue = numImageFilesProcessed
	stats.FrameFiles.PartValue = numImageFilesDeleted
	stats.FrameRecords.WholeValue = numFrameEntries
	stats.FrameRecords.PartValue = numFrameEntriesDeleted
	stats.ScoreRecords.WholeValue = numScoreEntries
	stats.ScoreRecords.PartValue = numScoreEntriesDeleted
	stats.Compacted = true
	return nil
}

// Dump state

func (state *BadgerBackedState) Dump(writer *bufio.Writer) error {
	// Dump frame information

	prefix := []byte(state.framePrefix)

	dbErr := state.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			filename := state.decodeFrameKey(item.Key())
			value, err := item.ValueCopy(nil)

			if err != nil {
				state.logger.Errorf("Failed to extract frameID for '%s': %s", filename, err)
				continue
			}

			frameID := state.decodeFrameValue(value)
			fmt.Fprintf(writer, "frame %6d  %20s  %s\n", frameID, state.GetFrameFile(frameID), filename)
		}

		return nil
	})

	if dbErr != nil {
		state.logger.Errorf("dumping frame information: %s", dbErr)
	}

	// Dump comparison score information

	dbErr = state.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(state.scorePrefix); it.ValidForPrefix(state.scorePrefix); it.Next() {
			item := it.Item()
			frameID1, frameID2 := state.decodeScoreKey(item.Key())
			value, err := item.ValueCopy(nil)

			if err != nil {
				state.logger.Errorf("Failed to extract score information for %d, %d: %s",
					frameID1, frameID2, err)
				continue
			}

			score, falsePositive := state.decodeScoreData(value)
			fmt.Fprintf(writer, "score %6d  %6d  %.4f  %v\n", frameID1, frameID2, score, falsePositive)
		}

		return nil
	})

	if dbErr != nil {
		state.logger.Errorf("dumping score information: %s", dbErr)
	}

	return dbErr
}

// Debug dump state

func (state *BadgerBackedState) DebugDump() {
}

func (state *BadgerBackedState) encodeFrameKey(path string) []byte {
	return []byte(state.framePrefix + path)
}

// Extract the filename from encoded frame key

func (state *BadgerBackedState) decodeFrameKey(encoded []byte) string {
	if state.prefixKeyLength < 0 {
		state.prefixKeyLength = len([]byte(state.framePrefix))
	}

	return string(encoded[state.prefixKeyLength:])
}

func (state *BadgerBackedState) encodeFrameValue(frameID int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(frameID))
	return key
}

func (state *BadgerBackedState) decodeFrameValue(encoded []byte) int {
	return int(binary.BigEndian.Uint64(encoded))
}

func (state *BadgerBackedState) encodeScoreKey(frameID1, frameID2 int) []byte {
	keyLen := len(state.scorePrefix) + 2*8 // prefix + 2 * uint64

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	key := make([]byte, keyLen)
	copy(key, state.scorePrefix)
	offset := len(state.scorePrefix)
	binary.BigEndian.PutUint64(key[offset:], uint64(frameID1))
	offset += 8
	binary.BigEndian.PutUint64(key[offset:], uint64(frameID2))
	return key
}

func (state *BadgerBackedState) decodeScoreKey(encoded []byte) (int, int) {
	prefixLen := len(state.scorePrefix)
	frameID1 := int(binary.BigEndian.Uint64(encoded[prefixLen : prefixLen+8]))
	frameID2 := int(binary.BigEndian.Uint64(encoded[prefixLen+8:]))
	return frameID1, frameID2
}

func (state *BadgerBackedState) encodeScoreData(score float32, falsePositive bool) ([]byte, error) {
	b := make([]byte, 5) // 4 bytes (float32) + 1 byte (bool)
	binary.BigEndian.PutUint32(b, math.Float32bits(score))
	b[4] = util.BoolToByte(falsePositive)
	return b, nil
}

func (state *BadgerBackedState) decodeScoreData(encoded []byte) (float32, bool) {
	score := math.Float32frombits(binary.BigEndian.Uint32(encoded[:4]))
	falsePositive := encoded[4] != 0
	return score, falsePositive
}

func (state *BadgerBackedState) getMaxFrameID() (int, error) {
	var maxFrameID int
	prefix := []byte(state.framePrefix)
	err := state.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Optimize for key-only iteration
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			// key := item.Key()

			// Extract integer from value bytes
			err := item.Value(func(val []byte) error {
				frameID := state.decodeFrameValue(val)

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
