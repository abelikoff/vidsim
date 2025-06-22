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

	"github.com/sirupsen/logrus"
)

func MakeLogger() *logrus.Logger {
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = "2006-01-02 15:04:05"
	formatter.FullTimestamp = true
	formatter.DisableColors = false
	logger.Formatter = formatter
	logger.Level = logrus.WarnLevel
	logger.Out = os.Stderr

	if *debugMode {
		logger.SetLevel(logrus.DebugLevel)
	} else if *verboseMode {
		logger.SetLevel(logrus.InfoLevel)
	}

	return logger
}
