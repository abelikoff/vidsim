package processor

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/abelikoff/vidsim/state"
	"github.com/sirupsen/logrus"
)

const (
	DefaultChrominanceTolerance = 0.3
	DefaultProportionTolerance  = 10.0
)

type Processor struct {
	numWorkers   int           // number of workers
	frames       []int         // list of all frame IDs we will be processing
	groups       map[int][]int // bucket -> list of frame IDs
	state        *state.State
	stats        StatsCollector
	logger       *logrus.Logger
	frameBuckets map[int]int    // frameID -> bucket
	nextBucket   int            // next bucket number
	exclusionRx  *regexp.Regexp // exclude files matching pattern
	bucketMutex  sync.Mutex
	QuietMode    bool          // be really quiet (only show warnings and errors)
	OutputWriter *bufio.Writer // where to write the report (nil means stdout)

	UseAbsolutePaths bool // When true filenames will be stored in the state with absolute paths.

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
	proc.state = state.MakeState()
	proc.nextBucket = 1
	proc.ChrTolerance = DefaultChrominanceTolerance
	proc.PropTolerance = DefaultProportionTolerance

	proc.bucketMutex = sync.Mutex{}

	err := proc.state.Init(stateDirectory, logger)

	if err != nil {
		logger.Fatalf("Failed to initialize state: %s", err)
	}

	proc.groups = make(map[int][]int)
	proc.frameBuckets = make(map[int]int)

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

	for ii := range numFiles {
		frameID1, found := proc.state.GetframeID(files[ii])

		if !found {
			proc.logger.Errorf("File '%s' is unknown", files[ii])
			failed = true
			continue
		}

		for jj := range ii {
			frameID2, found := proc.state.GetframeID(files[jj])

			if !found {
				proc.logger.Errorf("File '%s' is unknown", files[jj])
				failed = true
				continue
			}

			proc.state.UnmatchFrames(frameID1, frameID2, true)
		}
	}

	if failed {
		return errors.New("Failed to unmatch files")
	}

	return nil
}

// Perform state datastore compaction

func (proc *Processor) CompactState() error {
	return proc.state.CompactDataStore()
}

func (proc *Processor) ShowSummary() {
	proc.stats.ShowSummary()
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

func (proc *Processor) newBucket() int {
	bucket := proc.nextBucket
	proc.nextBucket++
	return bucket
}

func (proc *Processor) DebugDump() {
	proc.state.DebugDump()
	proc.logger.Debugf("--- groups ---------------------------\n%v\n", proc.groups)
}
