/*
Copyright © 2024 Alexander L. Belikoff <alexander@belikoff.net>
*/
package cmd

import (
	"bufio"
	"os"
	"runtime"

	"github.com/abelikoff/vidsim/processor"
	"github.com/abelikoff/vidsim/state"
	"github.com/spf13/cobra"
)

var clusteringMode *string        // Clustering mode
var externalFramegenPgm *string   // External program for frame generation
var externalComparisonPgm *string // External program for image comparison
var similarityThreshold *float32  // Custom similarity threshold
var chromTolerance *float64       // Chrominance tolerance flag
var propTolerance *float64        // Proportion tolerance flag
var useAbsolutePaths *bool        // Whether to store filenames with absolute paths
var ignoreFalsePositives *bool    // Tread false positives as matches

// processCmd represents the process command
var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Scan video files and report similar ones.",
	Long: `This command makes vidsim scan all video files in specified directories and reports those
it consideres similar. The report is output in JSON format.

`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := MakeLogger()
		nWorkers := *numWorkers

		if nWorkers <= 0 {
			nWorkers = runtime.NumCPU()
		}

		logger.Infof("Running with %d parallel workers", nWorkers)

		var clMode state.ClusteringMethod

		switch *clusteringMode {
		case "none":
			clMode = state.None

		case "union":
			clMode = state.Loose

		case "strict":
			clMode = state.Strict

		default:
			logger.Fatalf("Unknown clustering mode: %s", *clusteringMode)
		}

		proc := processor.MakeProcessor(nWorkers, *stateDirectory, clMode, logger)
		proc.ChrTolerance = *chromTolerance
		proc.PropTolerance = *propTolerance
		proc.UseAbsolutePaths = *useAbsolutePaths
		proc.IgnoreFalsePositives = *ignoreFalsePositives
		proc.ScorePrefix = *scorePrefix
		proc.ExternalFramegenTool = *externalFramegenPgm
		proc.ExternalComparisonTool = *externalComparisonPgm
		proc.DebugMode = *debugMode

		if *similarityThreshold < 0 || *similarityThreshold > 1 {
			logger.Fatalf("Bad similarity threshold: %f", *similarityThreshold)
		}

		proc.SimilarityThreshold = *similarityThreshold

		if *outputFile != "" {
			f, err := os.Create(*outputFile)

			if err != nil {
				logger.Fatalf("Cannot open output file '%s': %s", *outputFile, err)
			}

			proc.OutputWriter = bufio.NewWriter(f)
		}

		proc.QuietMode = *quietMode
		err := proc.SetExclusionPattern(*excludePattern)

		if err != nil {
			logger.Fatal("Processing failed")
		}

		err = proc.Process(args)

		if err != nil {
			logger.Fatal("Processing failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(processCmd)
	const DefaultSimilarityThreshold = 0.7

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// processCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// processCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	clusteringMode = processCmd.Flags().StringP("clustering_mode", "m", "strict",
		"Clustering mode (none, union, strict [default])")
	externalFramegenPgm = processCmd.Flags().StringP("framegen_tool", "G", "",
		"External frame generation tool")
	externalComparisonPgm = processCmd.Flags().StringP("comparison_tool", "C", "",
		"External image comparison tool")
	similarityThreshold = processCmd.Flags().Float32P("similarity_threshold", "T", DefaultSimilarityThreshold,
		"Lowest similarity score for images to be considered a match")
	useAbsolutePaths = processCmd.Flags().BoolP("abs_paths", "",
		false, "Store filenames with absolute paths")
	ignoreFalsePositives = processCmd.Flags().BoolP("ignore_false_positives", "",
		false, "Treat false positives as matches")
	chromTolerance = processCmd.Flags().Float64P("chr_tolerance", "",
		processor.DefaultChrominanceTolerance, "Chrominance tolerance level")
	propTolerance = processCmd.Flags().Float64P("prop_tolerance", "",
		processor.DefaultProportionTolerance, "Proportion tolerance level")
}
