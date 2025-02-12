/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"runtime"

	"github.com/abelikoff/vidsim/processor"
	"github.com/spf13/cobra"
)

// unmatchCmd represents the unmatch command
var unmatchCmd = &cobra.Command{
	Use:   "unmatch",
	Short: "Mark specified files as false positive match",
	Long: `Even though the frame comparison might yield a match, one can indicate that specified files

are not identical so that in future runs they would not be reported as a match.

This command only works with persistent state.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := MakeLogger()
		nWorkers := *numWorkers

		if nWorkers <= 0 {
			nWorkers = runtime.NumCPU()
		}

		logger.Infof("Running with %d parallel workers", nWorkers)
		proc := processor.MakeProcessor(nWorkers, *stateDirectory, logger)
		err := proc.Unmatch(args)

		if err != nil {
			logger.Fatal("Processing failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(unmatchCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// unmatchCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// unmatchCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
