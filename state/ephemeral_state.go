package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
			return nil, fmt.Errorf("Failed to create a temporary directory: %s", err)
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

func (state *EphemeralState) DeleteFile(path string) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	delete(state.image2frame, path)
}

func (state *EphemeralState) GetIDForFile(path string) (int, bool) {
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	frameID, found := state.image2frame[path]
	return frameID, found
}

func (state *EphemeralState) SetframeID(path string, frameID int) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	state.image2frame[path] = frameID
	state.frame2image[frameID] = path
}

func (state *EphemeralState) GetImageFile(frameID int) (string, bool) {
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	path, found := state.frame2image[frameID]
	return path, found
}

// Get name of the image frame file corresponding to the frame ID

func (state *EphemeralState) GetFrameFile(frameID int) string {
	return filepath.Join(state.dataDirectory, fmt.Sprintf("frame%06d.jpg", frameID))
}

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

func (state *EphemeralState) SetComparisonScore(frameID1 int, frameID2 int, score float32) {
	// make sure frame IDs are ordered

	if frameID1 > frameID2 {
		frameID2, frameID1 = frameID1, frameID2
	}

	key := [2]int{frameID1, frameID2}
	state.mutex.Lock()
	state.matchScores[key] = Match{Score: score, FalsePositive: false}
	state.mutex.Unlock()
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
