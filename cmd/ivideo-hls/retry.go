package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/appconfig"
	"github.com/chamrong/ivideo-hls/internal/pipeline"
)

func newRetryFailedCommand() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "retry-failed",
		Short: "Push per-video workspaces left behind by a failed run (no re-encoding)",
		Long: "Scans the working directory for hero_<name>/ workspaces that have a\n" +
			"committed playlist waiting to push. Re-running `ivideo-hls` would re-\n" +
			"encode these from scratch; retry-failed just force-pushes them, cleans\n" +
			"up on success, and leaves them alone on a second failure.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			loaded, _ := appconfig.Load()
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := buildConfig(wd, loaded)
			if err != nil {
				return err
			}
			candidates, err := pipeline.FindRetryCandidates(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return fmt.Errorf("scan workspaces: %w", err)
			}
			if len(candidates) == 0 {
				fmt.Println(styleOk.Render("✔ no pending retries — nothing to do"))
				return nil
			}
			printRetryCandidates(cfg, candidates)
			if !assumeYes && !confirm("Push these to "+cfg.RemoteURL+"?") {
				fmt.Println(styleDim("cancelled"))
				return nil
			}
			return runRetries(cmd.Context(), cfg, candidates)
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

var styleRetryRow = lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE"))

func printRetryCandidates(cfg *pipeline.Config, candidates []pipeline.RetryCandidate) {
	fmt.Printf("Found %d workspace%s with unpushed commits:\n\n",
		len(candidates), pluralS(len(candidates)))
	for _, c := range candidates {
		fmt.Printf("  %s  %s  %s\n",
			styleRetryRow.Render("✓ "+c.Name),
			styleDim("branch "+c.Branch),
			styleDim(humanBytes(c.Size)))
	}
	fmt.Println()
	fmt.Println(styleDim("Remote: ") + cfg.RemoteURL)
	fmt.Println()
}

func styleDim(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(s)
}

func confirm(question string) bool {
	fmt.Printf("%s [y/N] ", question)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func runRetries(ctx context.Context, cfg *pipeline.Config, candidates []pipeline.RetryCandidate) error {
	runner := pipeline.NewRunner(cfg, plainLogEmitter())
	var failed []string
	for _, c := range candidates {
		if err := runner.RetryOne(ctx, c); err != nil {
			fmt.Fprintln(os.Stderr, styleErr.Render("✗ "+c.Name+": "+err.Error()))
			failed = append(failed, c.Name)
			continue
		}
	}
	if len(failed) > 0 {
		fmt.Println(styleErr.Render(fmt.Sprintf("✗ %d of %d retries failed: %s",
			len(failed), len(candidates), strings.Join(failed, ", "))))
		os.Exit(1)
	}
	fmt.Println(styleOk.Render(fmt.Sprintf("✔ retried %d workspace%s",
		len(candidates), pluralS(len(candidates)))))
	return nil
}

// plainLogEmitter is the plain-log sink reused by the retry subcommand.
// Matches runPlain's shape: info/success/warn/error go to stdout/stderr
// with a short prefix; LevelDim is dropped for readability.
func plainLogEmitter() pipeline.Emitter {
	return pipeline.FuncEmitter(func(ev pipeline.Event) {
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
			// suppress for CLI readability
		default:
			fmt.Println(prefix + ev.Message)
		}
	})
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
