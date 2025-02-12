/*
Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
*/
package cmd

import (
	"bufio"
	"os"

	"github.com/abelikoff/vidsim/processor"
	"github.com/spf13/cobra"
)

// dumpCmd represents the dump command
var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump the state database",
	Run: func(_ *cobra.Command, args []string) {
		logger := MakeLogger()
		nWorkers := 1
		proc := processor.MakeProcessor(nWorkers, *stateDirectory, logger)

		if *outputFile != "" {
			f, err := os.Create(*outputFile)

			if err != nil {
				logger.Fatalf("Cannot open output file '%s': %s", *outputFile, err)
			}

			proc.OutputWriter = bufio.NewWriter(f)
		}

		err := proc.DumpState()

		if err != nil {
			logger.Fatalf("Dump failed: %s\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// compactCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// compactCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
