package video

import (
	"path/filepath"
	"regexp"
	"strings"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

type Quality string
type Compression string

const (
	QualityLow    Quality = "low"
	QualityMedium Quality = "medium"
	QualityHigh   Quality = "high"

	CompressionFast     Compression = "fast"
	CompressionBalanced Compression = "balanced"
	CompressionBest     Compression = "best"
)

type Video struct {
	Path   string
	Name   string
	Branch string
}

type Episode struct {
	Path   string
	Suffix string // "" for unsplit; "a","b",… for split parts
}

func NewVideo(path string) Video {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	branch := sanitizeRe.ReplaceAllString(name, "_")
	return Video{Path: path, Name: name, Branch: branch}
}
