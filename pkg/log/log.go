package log

import (
	"github.com/sirupsen/logrus"
)

var logger = logrus.New()

func StartLogger(json bool) {
	logger.SetFormatter(&logrus.TextFormatter{ForceColors: true, TimestampFormat: "2006-01-02 15:04:05", FullTimestamp: true})
	if json {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}
	logger.SetLevel(logrus.DebugLevel)
}

func Fatal(args ...interface{}) {
	logger.Fatal(args...)
}
func Fatalf(format string, args ...interface{}) {
	logger.Fatalf(format, args...)
}

func Warn(args ...interface{}) {
	logger.Warn(args...)
}
func Warnf(format string, args ...interface{}) {
	logger.Warnf(format, args...)
}

func Error(args ...interface{}) {
	logger.Error(args...)
}
func Errorf(format string, args ...interface{}) {
	logger.Errorf(format, args...)
}

func Info(args ...interface{}) {
	logger.Info(args...)
}
func Infof(format string, args ...interface{}) {
	logger.Infof(format, args...)
}

func Debug(args ...interface{}) {
	logger.Debug(args...)
}
func Debugf(format string, args ...interface{}) {
	logger.Debugf(format, args...)
}
