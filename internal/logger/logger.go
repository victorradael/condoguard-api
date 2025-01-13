package logger

import (
    "os"
    "github.com/sirupsen/logrus"
)

var log *logrus.Logger

func init() {
    log = logrus.New()
    
    // Set log format
    log.SetFormatter(&logrus.JSONFormatter{
        TimestampFormat: "2006-01-02 15:04:05",
        FieldMap: logrus.FieldMap{
            logrus.FieldKeyTime:  "timestamp",
            logrus.FieldKeyLevel: "severity",
            logrus.FieldKeyMsg:   "message",
        },
    })

    // Set output to stdout
    log.SetOutput(os.Stdout)

    // Set log level from environment variable or default to info
    level, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
    if err != nil {
        level = logrus.InfoLevel
    }
    log.SetLevel(level)
}

// Fields type for structured logging
type Fields map[string]interface{}

// Info logs an info level message
func Info(msg string, fields Fields) {
    if fields == nil {
        log.Info(msg)
        return
    }
    log.WithFields(logrus.Fields(fields)).Info(msg)
}

// Error logs an error level message
func Error(msg string, err error, fields Fields) {
    if fields == nil {
        fields = Fields{}
    }
    fields["error"] = err
    log.WithFields(logrus.Fields(fields)).Error(msg)
}

// Debug logs a debug level message
func Debug(msg string, fields Fields) {
    if fields == nil {
        log.Debug(msg)
        return
    }
    log.WithFields(logrus.Fields(fields)).Debug(msg)
}

// Warn logs a warning level message
func Warn(msg string, fields Fields) {
    if fields == nil {
        log.Warn(msg)
        return
    }
    log.WithFields(logrus.Fields(fields)).Warn(msg)
} 