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

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "vidsim",
	Short: "Identify similar videos",

	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// configuration options

var numWorkers *int            // number of parallel workers to use
var excludePattern *string     // pattern to exclude matching files
var excludePatternFile *string // file containing patterns to exclude matching files
var stateDirectory *string     // location of persistent state
var outputFile *string         // where to output the report
var scorePrefix *string        // Prefix to use for score records
var verboseMode *bool
var debugMode *bool
var quietMode *bool

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.vidsim.yaml)")
	numWorkers = rootCmd.PersistentFlags().IntP("workers", "P", 0,
		"number of parallel workers")
	stateDirectory = rootCmd.PersistentFlags().StringP("state_directory", "d", ".vidsim",
		"directory to store/use the state")
	scorePrefix = processCmd.Flags().StringP("score_prefix", "", "",
		"Prefix to use for score data")
	outputFile = rootCmd.PersistentFlags().StringP("output_file", "o", "",
		"file to output the report to")
	excludePattern = rootCmd.PersistentFlags().StringP("exclude", "x", "",
		"regex pattern to exclude matching files")
	excludePatternFile = rootCmd.PersistentFlags().StringP("exclude_file", "X", "",
		"file containing regex patterns to exclude matching files")
	verboseMode = rootCmd.PersistentFlags().BoolP("verbose", "v", false,
		"verbose mode")
	debugMode = rootCmd.PersistentFlags().BoolP("debug", "", false,
		"debug mode")
	quietMode = rootCmd.PersistentFlags().BoolP("quiet", "q", false,
		"quiet mode")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
