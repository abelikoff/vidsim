/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/abelikoff/vidsim/processor"
	"github.com/spf13/cobra"
)

// compactCmd represents the compact command
var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact the state database",
	Long: `Delete records corresponding to the files that no longer exist and compact the database.

IMPORTANT: Since the filenames are stored with relative paths, it is critical to run the compaction
from the same directory the original processing was run - otherwise all files in the store would be
considered non-existent and the store effectively wiped out (although the compation logic has
safety protection against such case).
`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := MakeLogger()
		nWorkers := 1
		proc := processor.MakeProcessor(nWorkers, *stateDirectory, logger)
		proc.ChrTolerance = *chromTolerance
		proc.PropTolerance = *propTolerance
		proc.QuietMode = *quietMode

		err := proc.CompactState()

		if err != nil {
			logger.Fatal("Compaction failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(compactCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// compactCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// compactCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
