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
