package processor

import (
	"errors"
	"fmt"
	"time"

	"github.com/schollz/progressbar/v3"
)

type StatsCollector struct {
	NumFilesToProcess   int
	NumFramesToGenerate int
	NumFramesGenerated  int
	NumTotalComparisons int
	NumComparisonsMade  int
	NumCacheHits        int
	NumMatches          int
	NumFalsePositives   int
	comparisonStartTime time.Time
	prevPercentage      int
	QuietMode           bool // don't show progress
	bar                 *progressbar.ProgressBar
}

func (stats *StatsCollector) IncNumFilesGenerated() {
	stats.NumFramesGenerated++

	if !stats.QuietMode {
		if stats.bar == nil {
			stats.bar = progressbar.Default(int64(stats.NumFilesToProcess), "Generating frames...")
		}

		stats.bar.Add(1)
	}
}

func (stats *StatsCollector) IncNumComparisonsMade() {
	if stats.comparisonStartTime.IsZero() {
		stats.comparisonStartTime = time.Now()

		if !stats.QuietMode {
			stats.bar = progressbar.Default(int64(stats.NumTotalComparisons), "Comparing frames...")
		}
	}

	stats.NumComparisonsMade++
	/* var eta string

	if etaSeconds, err := stats.EstimateCompletionETA(); err == nil {
		eta = fmt.Sprintf("ETA: %ds", etaSeconds)
	}

	percentageDone := int(float64(stats.NumComparisonsMade) / float64(stats.NumTotalComparisons) * 100) */

	if !stats.QuietMode {
		stats.bar.Add(1)
		/*if percentageDone > stats.prevPercentage {
			fmt.Printf("Done %d/%d (%d%%) comparisons    %s\n",
				stats.NumComparisonsMade, stats.NumTotalComparisons, percentageDone, eta)
			stats.prevPercentage = percentageDone
		}*/
	}
}

func (stats *StatsCollector) EstimateCompletionETA() (int, error) {
	now := time.Now()
	diff := now.Sub(stats.comparisonStartTime)
	const minDuration = 60 // minimal duration in seconds

	if diff.Seconds() < minDuration || stats.NumComparisonsMade == 0 {
		return 0, errors.New("not enough data to reliably estimate ETA")
	}

	eta := diff.Seconds() / float64(stats.NumComparisonsMade) * float64(stats.NumTotalComparisons-stats.NumComparisonsMade)
	return int(eta), nil
}

func (stats *StatsCollector) ShowSummary() {
	var genPercentage, compPercentage int

	if stats.NumFilesToProcess > 0 {
		genPercentage = int(float32(stats.NumFramesToGenerate) / float32(stats.NumFilesToProcess) * 100)
	}

	if stats.NumTotalComparisons > 0 {
		compPercentage = int(float32(stats.NumTotalComparisons-stats.NumCacheHits) / float32(stats.NumTotalComparisons) * 100)
	}

	fmt.Printf(`

SUMMARY
=======
Video files:         %10d
Frames generated:    %10d  (%d%%)
Total comparisons:   %10d
New comparisons:     %10d  (%d%%)
Total matches:       %10d
False positives:     %10d
`,
		stats.NumFilesToProcess,
		stats.NumFramesToGenerate,
		genPercentage,
		stats.NumTotalComparisons,
		stats.NumTotalComparisons-stats.NumCacheHits,
		compPercentage,
		stats.NumMatches,
		stats.NumFalsePositives)
}
