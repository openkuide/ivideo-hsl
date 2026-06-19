package scanner_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/adapters/secondary/scanner"
)

// TestScanner_Scan_EmptyDir verifies that Scan returns an empty slice (not
// nil error) when the root directory exists but contains no video files.
func TestScanner_Scan_EmptyDir(t *testing.T) {
	root := t.TempDir()
	s := scanner.New()
	videos, err := s.Scan(root, false)
	if err != nil {
		t.Fatalf("Scan(empty dir): %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("want 0 videos, got %d", len(videos))
	}
}

// TestScanner_Scan_NonExistentDir verifies that Scan returns an error when
// the root directory does not exist.
func TestScanner_Scan_NonExistentDir(t *testing.T) {
	s := scanner.New()
	_, err := s.Scan("/nonexistent/path/that/does/not/exist", false)
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}
