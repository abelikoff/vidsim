/*
Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
*/
package state

import (
	"bufio"

	"github.com/abelikoff/vidsim/util"
)

// State manager interface.

type State interface {
	AddFile(path string) (int, bool)                                        // Add file to the state and return its ID
	GetFileID(path string) (int, bool)                                      // Get ID corresponding to the video file
	GetVideoFile(ID int) (string, bool)                                     // Get video file for ID
	GetFrameFile(ID int) string                                             // Get frame file based on ID
	GetComparisonScore(ID1 int, ID2 int) (float32, bool, bool)              // Get comparison score for two files
	SetComparisonScore(ID1 int, ID2 int, score float32, falsePositive bool) // Set comparison score for two files
	ScanFiles(callback func(id int, path string) bool)                      // Scan all files in the state
	Compact(stats *util.CompactionStats) error                              // Compact the state
	Dump(writer *bufio.Writer) error                                        // Dump the state
	DebugDump()                                                             // Dump dump of the state
	Close()                                                                 // Close the state
}
