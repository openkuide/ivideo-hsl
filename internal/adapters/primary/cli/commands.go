// Package cli provides thin cobra command wrappers that delegate to *app.App.
// No pipeline logic lives here — only flag parsing, input validation, and
// calls to application-layer services.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

// Commands builds and returns the root cobra command wired to a.
func Commands(a *app.App) *cobra.Command {
	root := &cobra.Command{
		Use:   "ivideo-hls",
		Short: "Convert and publish videos as HLS to GitHub",
	}
	root.AddCommand(runCmd(a), retryCmd(a), resumeCmd(a), recoverCmd(a))
	return root
}

func runCmd(a *app.App) *cobra.Command {
	var (
		noPush   bool
		parallel int
	)
	cmd := &cobra.Command{
		Use:   "run [videos...]",
		Short: "Convert and push videos",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.Config.Load()
			if err != nil {
				return err
			}
			if noPush {
				cfg.Push = false
			}
			if parallel > 0 {
				cfg.MaxParallel = parallel
				cfg.ParallelMode = parallel > 1
			}
			var videos []video.Video
			if len(args) > 0 {
				for _, p := range args {
					videos = append(videos, video.NewVideo(p))
				}
			} else {
				scanned, err := a.Scanner.Scan(cfg.SourceDir, cfg.Recursive)
				if err != nil {
					return err
				}
				videos = scanned
			}
			if len(videos) == 0 {
				fmt.Fprintln(os.Stderr, "no videos found")
				return nil
			}
			results := a.Runner.Run(cmd.Context(), videos, cfg, nil)
			ok, fail := app.Summary(results)
			fmt.Printf("✓ %d ok · ✗ %d failed\n", ok, fail)
			if fail > 0 {
				fmt.Fprintln(os.Stderr, "  → run `ivideo-hls retry-failed` to push ready workspaces")
				fmt.Fprintln(os.Stderr, "  → run `ivideo-hls resume-failed` to re-encode broken workspaces")
				fmt.Fprintln(os.Stderr, "  → run `ivideo-hls recover` to triage and handle both at once")
				return fmt.Errorf("%d video(s) failed", fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noPush, "no-push", false, "commit but skip push")
	cmd.Flags().IntVarP(&parallel, "parallel", "j", 0, "number of parallel jobs")
	return cmd
}

func retryCmd(a *app.App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "retry-failed",
		Short: "Push workspaces that are ready (encoding done, push failed)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.Config.Load()
			if err != nil {
				return err
			}
			workspaces, err := a.Recovery.FindRetryReady(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
				fmt.Println("✔ no retry-ready workspaces")
				return nil
			}

			fmt.Printf("%d workspace(s) ready to push:\n\n", len(workspaces))
			for _, w := range workspaces {
				fmt.Printf("  ● %-24s  branch: %-32s  size: %s\n", w.Name, w.Branch, humanBytes(w.Size))
			}
			fmt.Println()

			if !yes && !confirm("Retry push?") {
				fmt.Println("aborted")
				return nil
			}

			pushURL := cfg.PushURL
			if pushURL == "" {
				return fmt.Errorf("no push URL configured — set one via settings")
			}

			ok, fail := 0, 0
			for _, w := range workspaces {
				err := a.Publishing.PushWorkspace(cmd.Context(), w.Workspace, w.Branch, pushURL)
				if err != nil {
					fmt.Printf("  ✗ %s — %s\n", w.Name, err)
					fail++
				} else {
					fmt.Printf("  ✓ %s → pushed\n", w.Name)
					ok++
				}
			}
			fmt.Printf("\n✓ %d ok · ✗ %d failed\n", ok, fail)
			if fail > 0 {
				return fmt.Errorf("%d workspace(s) failed to push", fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func resumeCmd(a *app.App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "resume-failed",
		Short: "Re-encode workspaces stuck mid-pipeline (source must exist)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.Config.Load()
			if err != nil {
				return err
			}
			incomplete, err := a.Recovery.FindIncomplete(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return err
			}
			if len(incomplete) == 0 {
				fmt.Println("✔ no incomplete workspaces")
				return nil
			}

			// Partition into recoverable and unrecoverable.
			var canResume, cannotResume int
			for _, w := range incomplete {
				src := "✓ source exists"
				if !w.SourceExists {
					src = "✗ source missing"
					cannotResume++
				} else {
					canResume++
				}
				hint := ""
				if w.Hint != "" {
					hint = "  · " + w.Hint
				}
				marker := "●"
				if !w.SourceExists {
					marker = "!"
				}
				fmt.Printf("  %s %-24s  stage: %-12s  %s%s\n",
					marker, w.Name, string(w.Stage), src, hint)
			}
			fmt.Println()

			if canResume == 0 {
				fmt.Println("no workspaces can be recovered (source files are missing)")
				return nil
			}

			fmt.Printf("%d of %d can be re-encoded.\n", canResume, len(incomplete))
			if !yes && !confirm("Resume?") {
				fmt.Println("aborted")
				return nil
			}

			ok, fail := 0, 0
			for _, w := range incomplete {
				if !w.SourceExists {
					continue
				}
				// Delete the partial workspace so the pipeline starts clean.
				if err := os.RemoveAll(w.Workspace); err != nil {
					fmt.Printf("  ✗ %s — could not remove workspace: %s\n", w.Name, err)
					fail++
					continue
				}
				results := a.Runner.Run(cmd.Context(), []video.Video{video.NewVideo(w.SourcePath)}, cfg, nil)
				if len(results) > 0 && results[0].Success {
					fmt.Printf("  ✓ %s → complete\n", w.Name)
					ok++
				} else {
					msg := "unknown error"
					if len(results) > 0 && results[0].Err != nil {
						msg = results[0].Err.Error()
					}
					fmt.Printf("  ✗ %s — %s\n", w.Name, msg)
					fail++
				}
			}
			fmt.Printf("\n✓ %d ok · ✗ %d failed\n", ok, fail)
			if fail > 0 {
				return fmt.Errorf("%d workspace(s) failed to resume", fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

// recoverCmd is a unified triage-and-action command that handles both retry-ready
// and incomplete workspaces in a single pass. It pushes the retry-ready set first
// (fast — no re-encoding), then re-encodes the incomplete set.
func recoverCmd(a *app.App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "recover",
		Short: "Triage and recover all failed workspaces (push-ready first, then re-encode)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.Config.Load()
			if err != nil {
				return err
			}

			retryReady, err := a.Recovery.FindRetryReady(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return fmt.Errorf("scan retry-ready: %w", err)
			}
			incomplete, err := a.Recovery.FindIncomplete(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return fmt.Errorf("scan incomplete: %w", err)
			}

			if len(retryReady) == 0 && len(incomplete) == 0 {
				fmt.Println("✔ nothing to recover")
				return nil
			}

			fmt.Println("Recovery triage:")
			fmt.Println()

			if len(retryReady) > 0 {
				fmt.Printf("  Push-ready (%d) — encoding done, just needs a push:\n", len(retryReady))
				for _, w := range retryReady {
					fmt.Printf("    ● %-24s  branch: %-32s  size: %s\n", w.Name, w.Branch, humanBytes(w.Size))
				}
				fmt.Println()
			}

			var resumable int
			if len(incomplete) > 0 {
				fmt.Printf("  Re-encode (%d) — stopped mid-pipeline:\n", len(incomplete))
				for _, w := range incomplete {
					src := "✓"
					if !w.SourceExists {
						src = "✗ source missing"
					} else {
						resumable++
					}
					hint := ""
					if w.Hint != "" {
						hint = "  · " + w.Hint
					}
					fmt.Printf("    ● %-24s  stage: %-12s  source: %s%s\n",
						w.Name, string(w.Stage), src, hint)
				}
				fmt.Println()
			}

			total := len(retryReady) + resumable
			if total == 0 {
				fmt.Println("no workspaces can be recovered (source files for incomplete jobs are missing)")
				return nil
			}

			fmt.Printf("%d action(s) will be taken.\n", total)
			if !yes && !confirm("Recover all?") {
				fmt.Println("aborted")
				return nil
			}

			ok, fail := 0, 0
			pushURL := cfg.PushURL

			// Phase 1: push retry-ready workspaces (fast).
			if len(retryReady) > 0 {
				fmt.Println("\nPushing...")
				if pushURL == "" {
					fmt.Println("  ⚠  no push URL configured — skipping push phase")
					fail += len(retryReady)
				} else {
					for _, w := range retryReady {
						if err := a.Publishing.PushWorkspace(cmd.Context(), w.Workspace, w.Branch, pushURL); err != nil {
							fmt.Printf("  ✗ %s — %s\n", w.Name, err)
							fail++
						} else {
							fmt.Printf("  ✓ %s → pushed\n", w.Name)
							ok++
						}
					}
				}
			}

			// Phase 2: re-encode incomplete workspaces.
			if resumable > 0 {
				fmt.Println("\nRe-encoding...")
				for _, w := range incomplete {
					if !w.SourceExists {
						continue
					}
					if err := os.RemoveAll(w.Workspace); err != nil {
						fmt.Printf("  ✗ %s — could not remove workspace: %s\n", w.Name, err)
						fail++
						continue
					}
					results := a.Runner.Run(cmd.Context(), []video.Video{video.NewVideo(w.SourcePath)}, cfg, nil)
					if len(results) > 0 && results[0].Success {
						fmt.Printf("  ✓ %s → complete\n", w.Name)
						ok++
					} else {
						msg := "unknown error"
						if len(results) > 0 && results[0].Err != nil {
							msg = results[0].Err.Error()
						}
						fmt.Printf("  ✗ %s — %s\n", w.Name, msg)
						fail++
					}
				}
			}

			fmt.Printf("\n✓ %d ok · ✗ %d failed\n", ok, fail)
			if fail > 0 {
				return fmt.Errorf("%d recovery action(s) failed", fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

// confirm prints prompt + " [y/N]: " and reads one line from stdin.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// humanBytes formats a byte count as a human-readable string.
func humanBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	const suffix = "KMGTPE"
	div := int64(1024)
	exp := 0
	for n := b / 1024; n >= 1024 && exp < len(suffix)-1; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), suffix[exp])
}
