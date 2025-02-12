/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"runtime"

	"github.com/abelikoff/vidsim/processor"
	"github.com/spf13/cobra"
)

// peekCmd represents the peek command
var peekCmd = &cobra.Command{
	Use:   "peek",
	Short: "Display state data",
	Long: `This command allows extract various information that is stored in the state data.

Usage:

    peek file <filename>                  - show file ID for the file
    peek score <filename> <filename>      - show match score for 2 files
    peek score <id> <id>                  - show match score for 2 files (represented by IDs)

This command only works with persistent state.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := MakeLogger()
		nWorkers := *numWorkers

		if nWorkers <= 0 {
			nWorkers = runtime.NumCPU()
		}

		logger.Infof("Running with %d parallel workers", nWorkers)
		proc := processor.MakeProcessor(nWorkers, *stateDirectory, logger)
		err := proc.Peek(args)

		if err != nil {
			logger.Fatal("Processing failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(peekCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// unmatchCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// unmatchCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
