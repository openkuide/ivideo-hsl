package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/appconfig"
	"github.com/chamrong/ivideo-hls/internal/deps"
	"github.com/chamrong/ivideo-hls/internal/pipeline"
	"github.com/chamrong/ivideo-hls/internal/tui"
)

const (
	envRemote = "IVIDEO_HLS_REMOTE"
	envToken  = "IVIDEO_HLS_TOKEN"
	envSource = "IVIDEO_HLS_SOURCE"
)

var (
	flagInputs      []string
	flagParallel    bool
	flagJobs        int
	flagQuality     string
	flagCompression string
	flagPreCompress bool
	flagAuto        bool
	flagRemote      string
	flagToken       string
	flagNoTUI       bool
	flagKeepSource  bool
	flagSettings    bool
	flagSource      string
	flagRecursive   bool
	flagNoPush      bool
	flagNoCleanup   bool
)

var (
	styleErr = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	styleOk  = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
)

func main() {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, styleErr.Render("error: "+err.Error()))
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          "ivideo-hls",
		Short:        "Convert multiple videos to HLS with git automation",
		Long:         banner() + "\n" + "A Go-powered HLS batch converter with a Bubble Tea TUI.",
		SilenceUsage: true,
		RunE:         run,
	}
	fs := root.Flags()
	fs.StringSliceVarP(&flagInputs, "input", "i", nil, "input .mp4 file (repeatable)")
	fs.BoolVarP(&flagParallel, "parallel", "p", false, "enable parallel processing")
	fs.IntVarP(&flagJobs, "jobs", "j", 0, "parallel jobs (implies -p when >1)")
	fs.StringVarP(&flagQuality, "quality", "q", "", "quality: low, medium, high")
	fs.StringVarP(&flagCompression, "compression", "c", "", "compression: fast, balanced, best")
	fs.BoolVar(&flagPreCompress, "pre-compress", false, "pre-compress before HLS")
	fs.BoolVar(&flagPreCompress, "compress", false, "deprecated alias for --pre-compress")
	_ = fs.MarkDeprecated("compress", "use --pre-compress instead")
	fs.BoolVarP(&flagAuto, "auto", "a", false, "auto-select all .mp4 in current directory")
	fs.StringVar(&flagRemote, "remote", "", "git remote URL (overrides config and $IVIDEO_HLS_REMOTE)")
	fs.StringVar(&flagToken, "token", "", "HTTPS auth token (overrides config and $IVIDEO_HLS_TOKEN)")
	fs.BoolVar(&flagNoTUI, "no-tui", false, "disable TUI (plain log output)")
	fs.BoolVar(&flagKeepSource, "keep-source", false, "keep the original .mp4 after a successful run")
	fs.StringVar(&flagSource, "source", "", "source directory to scan for video files (overrides config)")
	fs.BoolVarP(&flagRecursive, "recursive", "r", false, "scan subdirectories (skips .git, node_modules, hero*, hidden dirs)")
	fs.BoolVar(&flagNoPush, "no-push", false, "skip `git push` — commit locally, preserve workspace")
	fs.BoolVar(&flagNoCleanup, "no-cleanup", false, "keep per-video workspace on success (hero_*/)")
	fs.BoolVar(&flagSettings, "settings", false, "open the persistent-config editor and exit")
	root.AddCommand(newInstallDepsCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newRetryFailedCommand())
	root.AddCommand(newResumeFailedCommand())
	return root
}

func run(cmd *cobra.Command, _ []string) error {
	if err := checkPrereqs(); err != nil {
		return err
	}
	if flagSettings {
		return openSettings()
	}

	loaded, err := appconfig.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, styleErr.Render("warning: "+err.Error()+" — using built-in defaults"))
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := buildConfig(wd, loaded)
	if err != nil {
		return err
	}

	if err := ensureSourceDir(cfg, wd); err != nil {
		return err
	}

	if err := resolveVideoSelection(cfg, loaded); err != nil {
		return err
	}
	if len(cfg.Videos) == 0 {
		return fmt.Errorf("no videos selected")
	}
	slices.Sort(cfg.Videos)

	return executePipeline(cfg)
}

// ensureSourceDir creates cfg.SourceDir when it has been explicitly set (via
// flag, env, or config) and doesn't exist yet. When the source dir was left
// as the fallback (current working directory), we never create anything —
// "mkdir /tmp/videos because the user ran ivideo-hls from /tmp" would be
// surprising.
func ensureSourceDir(cfg *pipeline.Config, cwd string) error {
	if cfg.SourceDir == "" || cfg.SourceDir == cwd {
		return nil
	}
	info, err := os.Stat(cfg.SourceDir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("source path is not a directory: %s", cfg.SourceDir)
		}
		return nil
	case !os.IsNotExist(err):
		return fmt.Errorf("stat source dir: %w", err)
	}
	if err := os.MkdirAll(cfg.SourceDir, 0o755); err != nil {
		return fmt.Errorf("create source dir %s: %w", cfg.SourceDir, err)
	}
	fmt.Println(styleOk.Render("✔ created source directory: " + cfg.SourceDir))
	return nil
}

// buildConfig applies the precedence chain flag > env > file > built-in,
// producing a pipeline.Config ready to hand to the runner or TUI.
func buildConfig(wd string, loaded appconfig.File) (*pipeline.Config, error) {
	cfg := pipeline.NewConfig(wd)
	applyFileDefaults(cfg, loaded)
	applyEnv(cfg)
	if err := applyFlags(cfg); err != nil {
		return nil, err
	}
	cfg.PushURL = appconfig.EffectiveRemoteURL(cfg.RemoteURL, resolveToken(loaded), resolveAuthMethod(loaded))
	return cfg, nil
}

func applyFileDefaults(cfg *pipeline.Config, loaded appconfig.File) {
	if loaded.RemoteURL != "" {
		cfg.RemoteURL = loaded.RemoteURL
	}
	if loaded.DefaultQuality != "" && pipeline.ValidQuality(loaded.DefaultQuality) {
		cfg.Quality = pipeline.Quality(loaded.DefaultQuality)
	}
	if loaded.DefaultCompression != "" && pipeline.ValidCompression(loaded.DefaultCompression) {
		cfg.Compression = pipeline.Compression(loaded.DefaultCompression)
	}
	cfg.PreCompress = loaded.DefaultPreCompress
	cfg.KeepSource = loaded.DefaultKeepSource
	cfg.PublicURLPattern = loaded.PublicURLPattern
	if loaded.DefaultSourceDir != "" {
		cfg.SourceDir = loaded.DefaultSourceDir
	}
	cfg.Push = !loaded.DefaultPushDisabled
	cfg.Cleanup = !loaded.DefaultCleanupDisabled
	cfg.Recursive = loaded.DefaultRecursive
	if loaded.DefaultParallel > 1 {
		cfg.MaxParallel = loaded.DefaultParallel
		cfg.ParallelMode = true
	}
	cfg.ResumeReuseCompressed = loaded.ResumeReuseCompressed
}

func applyEnv(cfg *pipeline.Config) {
	if v := os.Getenv(envRemote); v != "" {
		cfg.RemoteURL = v
	}
	if v := os.Getenv(envSource); v != "" {
		cfg.SourceDir = v
	}
}

func applyFlags(cfg *pipeline.Config) error {
	if flagRemote != "" {
		cfg.RemoteURL = flagRemote
	}
	if flagSource != "" {
		abs, err := filepath.Abs(flagSource)
		if err != nil {
			return fmt.Errorf("resolve --source: %w", err)
		}
		cfg.SourceDir = abs
	}
	if flagPreCompress {
		cfg.PreCompress = true
	}
	if flagKeepSource {
		cfg.KeepSource = true
	}
	if flagNoPush {
		cfg.Push = false
	}
	if flagNoCleanup {
		cfg.Cleanup = false
	}
	if flagRecursive {
		cfg.Recursive = true
	}
	if flagQuality != "" {
		if !pipeline.ValidQuality(flagQuality) {
			return fmt.Errorf("invalid quality %q (low|medium|high)", flagQuality)
		}
		cfg.Quality = pipeline.Quality(flagQuality)
	}
	if flagCompression != "" {
		if !pipeline.ValidCompression(flagCompression) {
			return fmt.Errorf("invalid compression %q (fast|balanced|best)", flagCompression)
		}
		cfg.Compression = pipeline.Compression(flagCompression)
	}
	if flagJobs > 0 {
		cfg.MaxParallel = flagJobs
		cfg.ParallelMode = flagJobs > 1
	}
	if flagParallel {
		cfg.ParallelMode = true
		if cfg.MaxParallel < 2 {
			cfg.MaxParallel = 2
		}
	}
	return nil
}

func resolveToken(loaded appconfig.File) string {
	if flagToken != "" {
		return flagToken
	}
	if v := os.Getenv(envToken); v != "" {
		return v
	}
	return loaded.Token
}

func resolveAuthMethod(loaded appconfig.File) appconfig.AuthMethod {
	if loaded.AuthMethod != "" {
		return loaded.AuthMethod
	}
	return appconfig.InferAuthMethod(loaded.RemoteURL, appconfig.AuthSSH)
}

func resolveVideoSelection(cfg *pipeline.Config, loaded appconfig.File) error {
	switch {
	case flagAuto:
		files, err := pipeline.ScanVideos(cfg.SourceDir, cfg.Recursive)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("no video files in %s (extensions: %s)",
				cfg.SourceDir, strings.Join(pipeline.VideoExtensions, " "))
		}
		cfg.Videos = files
	case len(flagInputs) > 0:
		for _, in := range flagInputs {
			abs := in
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(cfg.SourceDir, in)
			}
			cfg.Videos = append(cfg.Videos, abs)
		}
	default:
		return runPickerFlow(cfg, loaded)
	}
	return nil
}

// runPickerFlow hands control to the TUI picker. When the user presses `s`
// it loops out to the settings screen and then back to the picker, so the
// operator can tweak config and return without restarting the CLI.
func runPickerFlow(cfg *pipeline.Config, loaded appconfig.File) error {
	firstRun := !appconfig.Exists()
	for {
		picker, err := tui.NewPicker(cfg)
		if err != nil {
			return err
		}
		if firstRun {
			picker.Banner = "👋 First run — press s to set your remote + token, or continue with defaults."
		}
		prog := teaNewProgram(picker)
		if _, err := prog.Run(); err != nil {
			return err
		}
		if picker.WantSettings {
			updated, err := openSettingsFrom(loaded)
			if err != nil {
				return err
			}
			loaded = updated
			reloadConfigFromFile(cfg, updated)
			firstRun = !appconfig.Exists() // if the user saved during settings, drop the banner
			continue
		}
		if !picker.Confirmed {
			fmt.Println(styleErr.Render("cancelled"))
			os.Exit(0)
		}
		// Persist run-config choices so the next session opens with the same
		// settings. Failures are non-fatal — the current run still proceeds.
		if err := appconfig.SaveRunConfig(
			string(cfg.Quality),
			string(cfg.Compression),
			cfg.MaxParallel,
			cfg.PreCompress,
			cfg.KeepSource,
		); err != nil {
			fmt.Fprintln(os.Stderr, styleErr.Render("warning: could not save run config: "+err.Error()))
		}
		return nil
	}
}

func reloadConfigFromFile(cfg *pipeline.Config, loaded appconfig.File) {
	applyFileDefaults(cfg, loaded)
	applyEnv(cfg)
	_ = applyFlags(cfg) // flags already validated once; ignore re-error
	cfg.PushURL = appconfig.EffectiveRemoteURL(cfg.RemoteURL, resolveToken(loaded), resolveAuthMethod(loaded))
}

func openSettings() error {
	loaded, err := appconfig.Load()
	if err != nil {
		return err
	}
	_, err = openSettingsFrom(loaded)
	return err
}

func openSettingsFrom(loaded appconfig.File) (appconfig.File, error) {
	path, err := appconfig.Path()
	if err != nil {
		return loaded, err
	}
	return tui.RunSettings(loaded, os.Getenv(envToken), path)
}

func executePipeline(cfg *pipeline.Config) error {
	if flagNoTUI {
		return runPlain(context.Background(), cfg)
	}
	results, err := tui.RunTUI(cfg)
	if err != nil {
		return err
	}
	ok, fail := pipeline.Summary(results)
	if fail == 0 {
		fmt.Println(styleOk.Render(fmt.Sprintf("✔ processed %d video%s", ok, pluralS(ok))))
		return nil
	}
	fmt.Println(styleErr.Render(fmt.Sprintf("✗ %d ok · %d failed", ok, fail)))
	os.Exit(1)
	return nil
}

func checkPrereqs() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH — install git (e.g. `brew install git` or `apt install git`)")
	}
	if err := checkFFmpegAvailable(); err != nil {
		return err
	}
	return nil
}

// checkFFmpegAvailable accepts ffmpeg/ffprobe either from deps (./bin or the
// user cache populated by `install-deps`) or from PATH. Fails with a clear
// hint when neither is available.
func checkFFmpegAvailable() error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		path := resolveBinary(bin)
		if _, err := exec.LookPath(path); err != nil {
			return fmt.Errorf(
				"%s not found. Install it with:\n"+
					"  ivideo-hls install-deps          (download a pinned static build)\n"+
					"  brew install ffmpeg              (macOS, system-wide)\n"+
					"  apt install ffmpeg               (Debian/Ubuntu)", bin)
		}
	}
	return nil
}

func resolveBinary(name string) string {
	switch name {
	case "ffmpeg":
		return deps.FFmpegPath()
	case "ffprobe":
		return deps.FFprobePath()
	}
	return name
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func banner() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true).Render("ivideo-hls")
}

func runPlain(ctx context.Context, cfg *pipeline.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	emitter := pipeline.FuncEmitter(func(ev pipeline.Event) {
		prefix := ""
		if ev.Job != "" {
			prefix = "[" + ev.Job + "] "
		}
		switch ev.Level {
		case pipeline.LevelError:
			fmt.Fprintln(os.Stderr, "✗ "+prefix+ev.Message)
		case pipeline.LevelWarn:
			fmt.Fprintln(os.Stderr, "! "+prefix+ev.Message)
		case pipeline.LevelSuccess:
			fmt.Println("✓ " + prefix + ev.Message)
		case pipeline.LevelDim:
			// skip to keep plain output readable
		default:
			fmt.Println(prefix + ev.Message)
		}
	})
	runner := pipeline.NewRunner(cfg, emitter)
	results := runner.Run(ctx)
	ok, fail := pipeline.Summary(results)
	if fail > 0 {
		return fmt.Errorf("%d ok, %d failed", ok, fail)
	}
	return nil
}
