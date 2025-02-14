package state

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/abelikoff/vidsim/util"
	"github.com/sirupsen/logrus"
)

type Match struct {
	Score         float32 // comparison score [0..1]
	FalsePositive bool    // true for false positives
}

// This is simple in-memory state, which is not persisted after the run.

type EphemeralState struct {
	dataDirectory string
	image2frame   map[string]int   // video filename -> frame ID
	frame2image   map[int]string   // frame ID -> video filename
	matchScores   map[[2]int]Match // pair of frame IDs (ordered numerically) -> match score information
	nextframeID   int

	mutex  *sync.RWMutex
	logger *logrus.Logger
}

func MakeEphemeralState(stateDirectory string, logger *logrus.Logger) (*EphemeralState, error) {
	state := new(EphemeralState)
	state.mutex = new(sync.RWMutex)
	state.image2frame = make(map[string]int)
	state.frame2image = make(map[int]string)
	state.matchScores = make(map[[2]int]Match)
	state.nextframeID = 1
	state.logger = logger

	if stateDirectory == "" {
		dirName, err := os.MkdirTemp("", "vidsim")

		if err != nil {
			return nil, fmt.Errorf("Failed to create a temporary directory: %w", err)
		}

		state.dataDirectory = dirName
	}

	if state.dataDirectory == "." {
		return nil, errors.New("don't use the current directory to keep the state")
	}

	return state, nil
}

func (state *EphemeralState) Close() {
	state.image2frame = make(map[string]int)
	state.frame2image = make(map[int]string)
	state.matchScores = make(map[[2]int]Match)
}

func (state *EphemeralState) AddFile(path string) (int, bool) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	var frameID int
	var found bool
	frameID, found = state.image2frame[path]

	if !found {
		frameID = state.nextframeID
		state.nextframeID++
		state.image2frame[path] = frameID
	}

	state.frame2image[frameID] = path
	return frameID, found
}

// Get ID corresponding to the video file

func (state *EphemeralState) GetFileID(path string) (int, bool) {
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	frameID, found := state.image2frame[path]
	return frameID, found
}

// Get video file for ID

func (state *EphemeralState) GetVideoFile(frameID int) (string, bool) {
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	path, found := state.frame2image[frameID]
	return path, found
}

// Get frame file based on ID

func (state *EphemeralState) GetFrameFile(frameID int) string {
	return filepath.Join(state.dataDirectory, fmt.Sprintf("frame%06d.jpg", frameID))
}

// Get comparison score for two files

func (state *EphemeralState) GetComparisonScore(frameID1 int, frameID2 int) (float32, bool, bool) {
	// make sure frame IDs are ordered

	if frameID1 > frameID2 {
		frameID2, frameID1 = frameID1, frameID2
	}

	key := [2]int{frameID1, frameID2}

	state.mutex.RLock()
	info, found := state.matchScores[key]
	state.mutex.RUnlock()

	if !found {
		return 0, false, found
	}

	return info.Score, info.FalsePositive, true
}

// Set comparison score for two files

func (state *EphemeralState) SetComparisonScore(frameID1 int, frameID2 int, score float32, falsePositive bool) {
	// make sure frame IDs are ordered

	if frameID1 > frameID2 {
		frameID2, frameID1 = frameID1, frameID2
	}

	key := [2]int{frameID1, frameID2}
	state.mutex.Lock()
	state.matchScores[key] = Match{Score: score, FalsePositive: falsePositive}
	state.mutex.Unlock()
}

// Dump the state

func (state *EphemeralState) Dump(writer *bufio.Writer) error {
	return nil
}

func (state *EphemeralState) DebugDump() {
	state.logger.Debugf("--- image2frame ------------------\n")

	for k, v := range state.image2frame {
		state.logger.Debugf("%s -> %d\n", k, v)
	}

	state.logger.Debugf("--- frame2image ------------------\n")

	for k, v := range state.frame2image {
		state.logger.Debugf("%d -> %s\n", k, v)
	}

	state.logger.Debugf("--- matchScores ------------------\n")

	for k, v := range state.matchScores {
		state.logger.Debugf("[%d, %d] -> {Score: %f, FalsePositive: %t}\n", k[0], k[1], v.Score, v.FalsePositive)
	}
}

// Compact the state

func (state *EphemeralState) Compact(stats *util.StatsCollector) error {
	return nil
}
