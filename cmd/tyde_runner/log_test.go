package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLogDir() string {
	return filepath.Join("testdata", "cache")
}

func TestLogPath(t *testing.T) {
	path := logPathRelativeTo(testLogDir())

	assert.Equal(t, "testdata/cache/fyne/com.fyshos.tyde/tyde.log", path)
}

func TestCrashLogPath(t *testing.T) {
	path := crashLogPathRelativeTo(testLogDir())

	assert.Contains(t, path, "testdata/cache/fyne/com.fyshos.tyde/tyde-crash-")
	assert.Contains(t, path, ".log")
}

func TestLogDir(t *testing.T) {
	dir := logDir(testLogDir())

	assert.Equal(t, "testdata/cache/fyne/com.fyshos.tyde", dir)
}
