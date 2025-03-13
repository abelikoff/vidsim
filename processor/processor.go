/*
Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
*/
package processor

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/abelikoff/vidsim/match"
	"github.com/abelikoff/vidsim/state"
	"github.com/abelikoff/vidsim/util"
	"github.com/sirupsen/logrus"
)

const (
	DefaultChrominanceTolerance = 0.3
	DefaultProportionTolerance  = 10.0
	DefaultSimilarityThreshold  = 0.7
)

type Processor struct {
	numWorkers             int   // number of workers
	frames                 []int // list of all frame IDs we will be processing
	state                  state.State
	stats                  util.StatsCollector
	logger                 *logrus.Logger
	clusterer              match.Clusterer // responsible for clustering the matches
	exclusionRx            *regexp.Regexp  // exclude files matching pattern
	bucketMutex            sync.Mutex
	QuietMode              bool                   // be really quiet (only show warnings and errors)
	ExternalFramegenTool   string                 // Program to use for frame generation
	ExternalComparisonTool string                 // Program to use for image comparison
	ClusteringMethod       match.ClusteringMethod // Clustering method
	OutputWriter           *bufio.Writer          // where to write the report (nil means stdout)
	SimilarityThreshold    float32                // images with match score above the threshold are considered a match

	ScorePrefix          string // Prefix to use for score records
	UseAbsolutePaths     bool   // When true filenames will be stored in the state with absolute paths
	IgnoreFalsePositives bool   // Treat false positives as matches
	CompactState         bool   // Whether to compact state after processing
	DebugMode            bool   // Enable debug mode

	// These two parameters govern the image comparison.
	// See https://pkg.go.dev/github.com/vitali-fedulov/images4@v1.3.1#CustomCoefficients for more details.
	//
	// In general, values < 1 mean more strict comparison, whereas > 1 means more lax one.

	ChrTolerance  float64 // Luma and Chrominance tolerance
	PropTolerance float64 // proportion tolerance
}

func MakeProcessor(numWorkers int, stateDirectory string, logger *logrus.Logger) *Processor {
	if numWorkers < 1 || numWorkers > 64 {
		logger.Fatalf("Bad number of workers: %d", numWorkers)
	}

	proc := new(Processor)
	proc.numWorkers = numWorkers
	proc.logger = logger
	proc.ChrTolerance = DefaultChrominanceTolerance
	proc.PropTolerance = DefaultProportionTolerance
	proc.SimilarityThreshold = DefaultSimilarityThreshold
	proc.ClusteringMethod = match.Strict

	proc.bucketMutex = sync.Mutex{}

	var err error
	//proc.state, err = state.MakeEphemeralState(stateDirectory, logger)
	proc.state, err = state.MakeBadgerBackedState(stateDirectory, logger, proc.ScorePrefix)

	if err != nil {
		logger.Fatalf("Failed to initialize state: %s", err)
	}

	return proc
}

func (proc *Processor) SetExclusionPattern(pattern string) error {
	if pattern == "" {
		return nil
	}

	var err error
	proc.exclusionRx, err = regexp.Compile(pattern)
	return err
}

func (proc *Processor) Process(directories []string) error {
	if len(directories) < 1 {
		proc.logger.Fatal("No directories passed")
	}

	if proc.clusterer == nil {
		proc.clusterer = match.NewClusterer(proc.ClusteringMethod, proc.logger)
	}

	proc.stats.QuietMode = proc.QuietMode
	canProceed := true

	for _, dir := range directories {
		info, err := os.Stat(dir)

		if err != nil || !info.IsDir() {
			proc.logger.Errorf("Not a proper directory: '%s'", dir)
			canProceed = false
		}
	}

	if !canProceed {
		return errors.New("bad parameters passed")
	}

	proc.stats.NumFilesToProcess = proc.countVideoFiles(directories)
	proc.generateFrames(directories)
	proc.compareFrames()
	proc.DebugDump()
	proc.GenerateReport()

	if proc.CompactState {
		proc.state.Compact(&proc.stats.Compaction)
	}

	proc.ShowSummary()
	return nil
}

func (proc *Processor) Unmatch(files []string) error {
	if len(files) < 2 {
		proc.logger.Fatal("Unmatching requires a list of files")
	}

	failed := false
	numFiles := len(files)

	for ii := 1; ii < numFiles; ii++ { // ii = 0 is meaningless because of the inner loop
		frameID1, found := proc.state.GetFileID(files[ii])

		if !found {
			proc.logger.Errorf("File '%s' is unknown", files[ii])
			failed = true
			continue
		}

		for jj := range ii {
			frameID2, found := proc.state.GetFileID(files[jj])

			if !found {
				proc.logger.Errorf("File '%s' is unknown", files[jj])
				failed = true
				continue
			}

			proc.state.SetComparisonScore(frameID1, frameID2, 0, true)
		}
	}

	if failed {
		return errors.New("Failed to unmatch files")
	}

	return nil
}

// Mark files specified as false positive vs all other files in the state.
func (proc *Processor) UnmatchAll(files []string) error {
	if len(files) < 1 {
		proc.logger.Fatal("Unmatching requires at least one file")
	}

	failed := false

	// Get all file IDs in the state
	allFileIDs := make(map[int]bool)

	// Collect all files that are going to be marked as false positives
	targetFiles := make(map[int]string)

	for _, file := range files {
		frameID, found := proc.state.GetFileID(file)

		if !found {
			proc.logger.Errorf("File '%s' is unknown", file)
			failed = true
			continue
		}

		targetFiles[frameID] = file
	}

	if failed {
		return errors.New("Failed to unmatch files: some files not found")
	}

	// For each file in the state, scan it to get all frameIDs
	proc.state.ScanFiles(func(id int, path string) bool {
		allFileIDs[id] = true
		return true
	})

	// Mark each target file as false positive with all other files
	for targetID := range targetFiles {
		for otherID := range allFileIDs {
			// Skip self-comparison
			if targetID == otherID {
				continue
			}

			proc.state.SetComparisonScore(targetID, otherID, 0, true)
		}
	}

	return nil
}

// Peek functionality

func (proc *Processor) Peek(args []string) error {
	if len(args) < 1 {
		proc.logger.Fatal("Bad peek syntax")
	}

	if len(args) == 2 && args[0] == "file" {
		/*if id, err := strconv.Atoi(args[1]); err == nil {
			filename, found := proc.state.GetVideoFile(id)

			if !found {
				proc.logger.Fatalf("File with ID %d is unknown", id)
			}

			fmt.Printf("%d  =>  '%s'\n", id, filename)
		} else { */
		filename := args[1]
		id, found := proc.state.GetFileID(filename)

		if !found {
			proc.logger.Fatalf("File '%s' is unknown", filename)
		}

		fmt.Printf("'%s'  =>  %d\n", filename, id)
		// }
	} else if len(args) == 3 && args[0] == "score" {
		var id1, id2 int
		var err error

		if id1, err = strconv.Atoi(args[1]); err != nil {
			id1 = -1
		}

		if id2, err = strconv.Atoi(args[2]); err != nil {
			id2 = -1
		}

		if id1 >= 0 && id2 >= 0 {
			score, falsePositive, found := proc.state.GetComparisonScore(id1, id2)

			if !found {
				proc.logger.Fatalf("Score for %d / %d not found", id1, id2)
			}

			fmt.Printf("%d %d  => %.4f  %v\n", id1, id2, score, falsePositive)
		} else if id1 < 0 && id2 < 0 {
			id1, found := proc.state.GetFileID(args[1])

			if !found {
				proc.logger.Fatalf("File '%s' is unknown", args[1])
			}

			id2, found := proc.state.GetFileID(args[2])

			if !found {
				proc.logger.Fatalf("File '%s' is unknown", args[2])
			}

			score, falsePositive, found := proc.state.GetComparisonScore(id1, id2)

			if !found {
				proc.logger.Fatalf("Score for %d / %d not found", id1, id2)
			}

			fmt.Printf("%d %d  => %.4f  %v\n", id1, id2, score, falsePositive)
		} else {
			proc.logger.Fatal("Arguments should be both either files or IDs")
		}

	} else {
		proc.logger.Fatal("Bad peek syntax")
	}

	return nil
}

// Dump the state

func (proc *Processor) DumpState() error {
	var writer *bufio.Writer

	if proc.OutputWriter != nil {
		writer = proc.OutputWriter
	} else {
		writer = bufio.NewWriter(os.Stdout)
	}

	defer writer.Flush()
	return proc.state.Dump(writer)
}

func (proc *Processor) ShowSummary() {
	if err := proc.stats.ShowSummary(); err != nil {
		proc.logger.Errorf("inconsistent statistics: %s", err)
	}
}

func (proc *Processor) countVideoFiles(directories []string) int {
	var numFiles int

	for _, dir := range directories {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && proc.isEligibleFile(path) {
				numFiles++
			}

			return nil
		})
	}

	return numFiles
}

func (proc *Processor) DebugDump() {
	if !proc.DebugMode {
		return
	}

	proc.state.DebugDump()
	proc.clusterer.DebugDump()
}
