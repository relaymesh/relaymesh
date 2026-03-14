package core

import (
	"log"
	"os"
	"strings"
)

// NewLogger creates a new logger with a standardized prefix.
// The prefix will be "github.com/relaymesh/relaymesh" or "github.com/relaymesh/relaymesh/<component>" if a component name is provided.
func NewLogger(component string) *log.Logger {
	prefix := "github.com/relaymesh/relaymesh"
	if component != "" {
		prefix = prefix + "/" + component
	}
	return log.New(os.Stdout, prefix+" ", log.LstdFlags|log.Lmicroseconds)
}

func WithRequestID(base *log.Logger, requestID string) *log.Logger {
	if base == nil {
		base = log.Default()
	}
	prefix := strings.TrimSpace(base.Prefix())
	if requestID != "" {
		if prefix != "" {
			prefix = prefix + " "
		}
		prefix = prefix + "request_id=" + requestID
	}
	return log.New(base.Writer(), prefix+" ", base.Flags())
}
