package deps

import (
	"archive/tar"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// InstallOptions tunes installation behavior.
type InstallOptions struct {
	// Force reinstalls even if the cached file already exists.
	Force bool
	// Progress is invoked with short human-readable status strings.
	Progress func(msg string)
	// SkipChecksum bypasses SHA256 verification. Intended only for
	// bootstrapping a fresh sources table; never use in production.
	SkipChecksum bool
}

// Install downloads and installs ffmpeg + ffprobe for the current platform.
func Install(ctx context.Context, opts InstallOptions) error {
	report := opts.Progress
	if report == nil {
		report = func(string) {}
	}
	for _, b := range []Binary{FFmpeg, FFprobe} {
		if err := installOne(ctx, b, opts, report); err != nil {
			return fmt.Errorf("install %s: %w", b, err)
		}
	}
	return nil
}

func installOne(ctx context.Context, b Binary, opts InstallOptions, report func(string)) error {
	target, err := targetPath(b)
	if err != nil {
		return err
	}
	if !opts.Force && isExecutable(target) {
		report(fmt.Sprintf("%s: already installed (%s)", b, target))
		return nil
	}

	src, err := sourceFor(b)
	if err != nil {
		return err
	}
	if src.SHA256 == "" && !opts.SkipChecksum {
		return fmt.Errorf(
			"%s: no pinned SHA256 for %s/%s — fetch the current one with:\n"+
				"  curl -fsSL %s | shasum -a 256\n"+
				"then update internal/infrastructure/deps/sources.go (or pass --skip-checksum to bypass, for dev only)",
			b, runtime.GOOS, runtime.GOARCH, src.URL)
	}

	report(fmt.Sprintf("%s: downloading %s", b, src.URL))
	body, err := download(ctx, src.URL)
	if err != nil {
		return err
	}
	defer os.Remove(body)

	if src.SHA256 != "" {
		report(fmt.Sprintf("%s: verifying checksum", b))
		if err := verifySHA256(body, src.SHA256); err != nil {
			return err
		}
	}

	report(fmt.Sprintf("%s: extracting → %s", b, target))
	if err := extract(body, src, string(b), target); err != nil {
		return err
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	report(fmt.Sprintf("%s: installed", b))
	return nil
}

func targetPath(b Binary) (string, error) {
	dir, err := CacheBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	name := string(b)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name), nil
}

// download fetches url to a temp file and returns its path. Caller deletes.
func download(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %s: %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp("", "ivideo-hls-download-*.bin")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

func extract(archivePath string, src source, innerBaseName, target string) error {
	switch src.Archive {
	case "raw":
		return moveFile(archivePath, target)
	case "zip":
		return extractZip(archivePath, innerBaseName, target)
	case "tar.xz":
		return extractTarXz(archivePath, innerBaseName, target)
	}
	return fmt.Errorf("unknown archive type %q", src.Archive)
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// cross-device move — fall back to copy+delete
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func extractZip(archivePath, innerBaseName, target string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if path.Base(f.Name) != innerBaseName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeFile(target, rc)
	}
	return fmt.Errorf("no entry named %q inside archive", innerBaseName)
}

func extractTarXz(archivePath, innerBaseName, target string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("open xz: %w", err)
	}
	tr := tar.NewReader(xzr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("no entry named %q inside tar.xz", innerBaseName)
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if path.Base(h.Name) != innerBaseName {
			continue
		}
		return writeFile(target, tr)
	}
}

func writeFile(target string, r io.Reader) error {
	out, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}
