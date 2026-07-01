package ui

import (
	"os"
	"testing"
)

// TestMain disables the package's background goroutines for tests.
func TestMain(m *testing.M) {
	startClock = func(*widgetPanel) {}

	os.Exit(m.Run())
}
