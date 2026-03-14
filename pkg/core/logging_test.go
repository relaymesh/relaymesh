package core

import (
	"io"
	"log"
	"testing"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger("")
	if logger.Prefix() != "github.com/relaymesh/relaymesh " {
		t.Fatalf("expected base prefix, got %q", logger.Prefix())
	}

	component := NewLogger("server")
	if component.Prefix() != "github.com/relaymesh/relaymesh/server " {
		t.Fatalf("expected component prefix, got %q", component.Prefix())
	}
}

func TestWithRequestID(t *testing.T) {
	base := log.New(io.Discard, "github.com/relaymesh/relaymesh/server ", 0)
	logger := WithRequestID(base, "req-123")
	if logger.Prefix() != "github.com/relaymesh/relaymesh/server request_id=req-123 " {
		t.Fatalf("unexpected prefix: %q", logger.Prefix())
	}

	logger = WithRequestID(base, "")
	if logger.Prefix() != "github.com/relaymesh/relaymesh/server " {
		t.Fatalf("expected base prefix, got %q", logger.Prefix())
	}
}
