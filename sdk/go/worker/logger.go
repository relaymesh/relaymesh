package worker

import (
	"log"
	"os"
)

// Logger is a simple interface for logging.
type Logger interface {
	Printf(format string, args ...interface{})
}

// stdLogger is the default logger, which uses the standard log package.
type stdLogger struct{}

// Printf prints a formatted message to the default worker logger.
func (stdLogger) Printf(format string, args ...interface{}) {
	defaultWorkerLogger.Printf(format, args...)
}

var defaultWorkerLogger = log.New(os.Stdout, "github.com/relaymesh/relaymesh/worker ", log.LstdFlags|log.Lmicroseconds)
