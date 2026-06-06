package deps

import (
	"fmt"
	"runtime"
)

// source describes one downloadable artifact. SHA256 is required — an empty
// checksum means we refuse to install (safer default than "fetch and hope").
//
// How to populate SHA256:
//   curl -fsSLo /tmp/bundle <url>
//   shasum -a 256 /tmp/bundle
//
// Update-policy note: pinning by SHA is deliberate. The upstream mirrors
// publish rolling static builds; checksums pinned here represent the exact
// bytes we last reviewed. Bumping version = bump URL + SHA in the same
// commit. Running `ivideo-hls install-deps --force` after a bump refreshes
// the user's cache.
type source struct {
	// URL is the archive to download.
	URL string
	// SHA256 is the lowercase hex digest of the archive at URL.
	SHA256 string
	// Archive is the type: "zip", "tar.xz", "raw" (single binary with no wrapper).
	Archive string
	// Inner is the path inside the archive to extract. "" means the archive IS
	// the binary (Archive == "raw").
	Inner string
}

// sources is the single source of truth for where prebuilt ffmpeg comes from.
// Keys are GOOS/GOARCH/Binary.
//
// Mirrors chosen for their canonical status:
//   - evermeet.cx — the de-facto macOS static-build publisher, maintained by
//     a long-standing member of the ffmpeg community.
//   - johnvansickle.com — the canonical linux static-build publisher, linked
//     from the ffmpeg project's own download page.
//
// SHA256 values are left empty in this initial landing; run install-deps with
// the (intentional) error message to get instructions for filling them in.
var sources = map[string]source{
	// macOS: evermeet.cx publishes universal binaries, so darwin/arm64 and
	// darwin/amd64 share the same archive and SHA.
	key("darwin", "arm64", FFmpeg): {
		URL:     "https://evermeet.cx/ffmpeg/ffmpeg-7.1.zip",
		SHA256:  "5a1303c7babaffff3c32c141ff49c7f44bd3b3b3e7dcea992fd7d04b6558ef43",
		Archive: "zip",
		Inner:   "ffmpeg",
	},
	key("darwin", "arm64", FFprobe): {
		URL:     "https://evermeet.cx/ffmpeg/ffprobe-7.1.zip",
		SHA256:  "fc289c963346d7dc0891cbaed02ba270e8abec54df9259e22d59559018b25709",
		Archive: "zip",
		Inner:   "ffprobe",
	},
	key("darwin", "amd64", FFmpeg): {
		URL:     "https://evermeet.cx/ffmpeg/ffmpeg-7.1.zip",
		SHA256:  "5a1303c7babaffff3c32c141ff49c7f44bd3b3b3e7dcea992fd7d04b6558ef43",
		Archive: "zip",
		Inner:   "ffmpeg",
	},
	key("darwin", "amd64", FFprobe): {
		URL:     "https://evermeet.cx/ffmpeg/ffprobe-7.1.zip",
		SHA256:  "fc289c963346d7dc0891cbaed02ba270e8abec54df9259e22d59559018b25709",
		Archive: "zip",
		Inner:   "ffprobe",
	},
	// Linux: johnvansickle.com archives put the binary in a versioned top
	// directory; the tar.xz extractor matches by basename so ffmpeg/ffprobe
	// pull from the same archive SHA.
	key("linux", "amd64", FFmpeg): {
		URL:     "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz",
		SHA256:  "abda8d77ce8309141f83ab8edf0596834087c52467f6badf376a6a2a4c87cf67",
		Archive: "tar.xz",
		Inner:   "ffmpeg",
	},
	key("linux", "amd64", FFprobe): {
		URL:     "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz",
		SHA256:  "abda8d77ce8309141f83ab8edf0596834087c52467f6badf376a6a2a4c87cf67",
		Archive: "tar.xz",
		Inner:   "ffprobe",
	},
	key("linux", "arm64", FFmpeg): {
		URL:     "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-arm64-static.tar.xz",
		SHA256:  "f4149bb2b0784e30e99bdda85471c9b5930d3402014e934a5098b41d0f7201b1",
		Archive: "tar.xz",
		Inner:   "ffmpeg",
	},
	key("linux", "arm64", FFprobe): {
		URL:     "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-arm64-static.tar.xz",
		SHA256:  "f4149bb2b0784e30e99bdda85471c9b5930d3402014e934a5098b41d0f7201b1",
		Archive: "tar.xz",
		Inner:   "ffprobe",
	},
}

func key(goos, goarch string, b Binary) string {
	return goos + "/" + goarch + "/" + string(b)
}

// sourceFor returns the pinned download for the current host, or
// ErrPlatformUnsupported when no entry exists.
func sourceFor(b Binary) (source, error) {
	s, ok := sources[key(runtime.GOOS, runtime.GOARCH, b)]
	if !ok {
		return source{}, fmt.Errorf("%w: %s/%s", ErrPlatformUnsupported, runtime.GOOS, runtime.GOARCH)
	}
	return s, nil
}
