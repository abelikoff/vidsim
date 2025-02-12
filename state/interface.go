package state

// State manager interface.

type StateNew interface {
	Close()                                                                 // Close the state
	AddFile(path string) (int, bool)                                        // Add file to the state and return its ID
	DeleteFile(path string)                                                 // Delete file from the state
	GetIDForFile(path string) (int, bool)                                   // Get ID for  file
	GetFileForID(ID int) (string, bool)                                     // Get file for ID
	GetFrameFile(ID int) string                                             // Get frame file based on ID
	GetComparisonScore(ID1 int, ID2 int) (float32, bool, bool)              // Get comparison score for two  files
	SetComparisonScore(ID1 int, ID2 int, score float32, falsePositive bool) // Set comparison score for two  files
	Compact() error                                                         // Compact the state
	DebugDump()                                                             // Dump the state
}
