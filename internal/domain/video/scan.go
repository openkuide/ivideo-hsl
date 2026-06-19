package video

import (
	"io/fs"
	"path/filepath"
	"strings"
)

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true,
	".avi": true, ".webm": true, ".m4v": true,
}

var ignoredDirs = map[string]bool{
	"node_modules": true, "vendor": true,
}

func IsVideoFile(name string) bool {
	return videoExts[strings.ToLower(filepath.Ext(name))]
}

// IsIgnoredDir reports whether a directory name should be skipped during
// video discovery. Exported so filesystem adapters can reuse domain rules.
func IsIgnoredDir(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "hero_") {
		return true
	}
	return ignoredDirs[name]
}

// ScanVideos filters entries for video files. Pass recursive=false for flat
// scan (entries are the root dir's children); recursive=true is handled by
// the adapter which walks the tree and calls ScanVideos per directory.
func ScanVideos(entries []fs.DirEntry, root string, recursive bool) []Video {
	var out []Video
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !IsVideoFile(e.Name()) {
			continue
		}
		out = append(out, NewVideo(filepath.Join(root, e.Name())))
	}
	return out
}
