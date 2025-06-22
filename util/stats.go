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

package util

import (
	"errors"
	"fmt"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Convenince type to represent fractions and percentages for stats

type Fraction struct {
	WholeValue int
	PartValue  int
}

func (frac *Fraction) IntPercentage() int {
	if frac.WholeValue > 0 {
		return int(float64(frac.PartValue) / float64(frac.WholeValue) * 100.0)
	}

	return -1000
}

// Compaction-related stats

type CompactionStats struct {
	Compacted    bool     // True when compaction occured
	FrameRecords Fraction // Frame records stats
	ScoreRecords Fraction // Score records stats
	FrameFiles   Fraction // Frame files stats
}

type StatsCollector struct {
	NumFilesToProcess   int
	NumFramesToGenerate int
	NumFramesGenerated  int
	NumTotalComparisons int
	NumComparisonsMade  int
	NumMismatches       int
	NumCacheHits        int
	NumMatches          int
	NumFalsePositives   int
	Compaction          CompactionStats
	comparisonStartTime time.Time
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
		stats.bar.Describe(fmt.Sprintf("Comparing frames...[%d new/%d matches]  ",
			stats.NumComparisonsMade-stats.NumCacheHits, stats.NumMatches))
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

func (stats *StatsCollector) ShowSummary() error {
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
Mismatches:          %10d
Total matches:       %10d
False positives:     %10d`,
		stats.NumFilesToProcess,
		stats.NumFramesToGenerate,
		genPercentage,
		stats.NumTotalComparisons,
		stats.NumTotalComparisons-stats.NumCacheHits,
		compPercentage,
		stats.NumMismatches,
		stats.NumMatches,
		stats.NumFalsePositives)

	if stats.NumFilesToProcess*(stats.NumFilesToProcess-1)/2 != stats.NumTotalComparisons {
		return errors.New("number of comparisons inconsistent with number of files")
	}

	if stats.NumMismatches+stats.NumMatches+stats.NumFalsePositives != stats.NumTotalComparisons {
		return errors.New("number of comparisons inconsistent match statistics")
	}

	if stats.Compaction.Compacted {
		fmt.Printf(`
Frames deleted:      %10d  (%d%%)
Scores deleted:      %10d  (%d%%)
Images deleted:      %10d  (%d%%)
`,
			stats.Compaction.FrameRecords.PartValue,
			stats.Compaction.FrameRecords.IntPercentage(),
			stats.Compaction.ScoreRecords.PartValue,
			stats.Compaction.ScoreRecords.IntPercentage(),
			stats.Compaction.FrameFiles.PartValue,
			stats.Compaction.FrameFiles.IntPercentage())
	} else {
		fmt.Println("")
	}

	return nil
}
