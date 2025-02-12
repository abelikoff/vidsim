/*
Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
*/
package processor

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/abelikoff/vidsim/match"
	"github.com/abelikoff/vidsim/state"
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
	stats                  StatsCollector
	logger                 *logrus.Logger
	clusterer              match.Clusterer // responsible for clustering the matches
	exclusionRx            *regexp.Regexp  // exclude files matching pattern
	bucketMutex            sync.Mutex
	QuietMode              bool          // be really quiet (only show warnings and errors)
	ExternalFramegenTool   string        // Program to use for frame generation
	ExternalComparisonTool string        // Program to use for image comparison
	OutputWriter           *bufio.Writer // where to write the report (nil means stdout)
	SimilarityThreshold    float32       // images with similarity score above the threshold are considered a match

	ScorePrefix          string // Prefix to use for score records
	UseAbsolutePaths     bool   // When true filenames will be stored in the state with absolute paths
	IgnoreFalsePositives bool   // Treat false positives as matches
	DebugMode            bool   // Enable debug mode

	// These two parameters govern the image comparison.
	// See https://pkg.go.dev/github.com/vitali-fedulov/images4@v1.3.1#CustomCoefficients for more details.
	//
	// In general, values < 1 mean more strict comparison, whereas > 1 means more lax one.

	ChrTolerance  float64 // Luma and Chrominance tolerance
	PropTolerance float64 // proportion tolerance
}

func MakeProcessor(numWorkers int, stateDirectory string, clusteringMode match.ClusteringMethod, logger *logrus.Logger) *Processor {
	if numWorkers < 1 || numWorkers > 64 {
		logger.Fatalf("Bad number of workers: %d", numWorkers)
	}

	proc := new(Processor)
	proc.numWorkers = numWorkers
	proc.logger = logger
	proc.ChrTolerance = DefaultChrominanceTolerance
	proc.PropTolerance = DefaultProportionTolerance
	proc.SimilarityThreshold = DefaultSimilarityThreshold

	proc.bucketMutex = sync.Mutex{}

	var err error
	//proc.state, err = state.MakeEphemeralState(stateDirectory, logger) // TODO: implement scorePrefix
	proc.state, err = state.MakeBadgerBackedState(stateDirectory, logger, proc.ScorePrefix)

	if err != nil {
		logger.Fatalf("Failed to initialize state: %s", err)
	}

	proc.clusterer = match.NewClusterer(clusteringMode, logger)
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

// Perform state datastore compaction

func (proc *Processor) CompactState() error {
	return proc.state.Compact()
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
