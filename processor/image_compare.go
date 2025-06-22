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

package processor

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vitali-fedulov/images4"
)

const (
	ScoreSimilar   float32 = 1 // score to assign for similar images
	ScoreDifferent float32 = 0 // score to assign for different images
)

func (proc *Processor) compareImageFiles(imageFile1 string, imageFile2 string) (float32, error) {
	if proc.ExternalComparisonTool == "" {
		return proc.compareImageFilesUsingInternal(imageFile1, imageFile2)
	} else {
		return proc.compareImageFilesUsingExternal(imageFile1, imageFile2)
	}
}

func (proc *Processor) compareImageFilesUsingInternal(imageFile1 string, imageFile2 string) (float32, error) {
	img1, err := images4.Open(imageFile1)

	if err != nil {
		proc.logger.Errorf("Failed to open image file %s: %v", imageFile1, err)
		return 0, err
	}

	img2, err := images4.Open(imageFile2)

	if err != nil {
		proc.logger.Errorf("Failed to open image file %s: %v", imageFile2, err)
		return 0, err
	}

	// Icons are compact hash-like image representations.

	icon1 := images4.Icon(img1)
	icon2 := images4.Icon(img2)

	if images4.CustomSimilar(icon1, icon2,
		images4.CustomCoefficients{Y: proc.ChrTolerance, Cb: proc.ChrTolerance, Cr: proc.ChrTolerance, Prop: proc.PropTolerance}) {
		proc.logger.Debugf("SIMILAR: %s and %s", imageFile1, imageFile2)
		return ScoreSimilar, nil
	}

	return ScoreDifferent, nil
}

func (proc *Processor) compareImageFilesUsingExternal(imageFile1 string, imageFile2 string) (float32, error) {

	cmd := exec.Command(proc.ExternalComparisonTool, imageFile1, imageFile2)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	err := cmd.Run()

	if err != nil {
		proc.logger.Errorf("Failed to run external command (%s %s %s): %v", proc.ExternalComparisonTool, imageFile1, imageFile2, err)
		return 0, err
	}

	output := strings.TrimSuffix(outBuf.String(), "\n")
	floatValue, err := strconv.ParseFloat(output, 32)

	if err != nil || floatValue < 0 || floatValue > 1 {
		proc.logger.Errorf("Bad comparison score '%s' from (%s %s %s): %v",
			output, proc.ExternalComparisonTool, imageFile1, imageFile2, err)
		return 0, err
	}

	return float32(floatValue), nil
}
