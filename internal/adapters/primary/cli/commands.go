// Package cli provides thin cobra command wrappers that delegate to *app.App.
// No pipeline logic lives here — only flag parsing, input validation, and
// calls to application-layer services.
package cli

import (
	"fmt"
	"os"

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
	root.AddCommand(runCmd(a), retryCmd(a), resumeCmd(a))
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
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noPush, "no-push", false, "commit but skip push")
	cmd.Flags().IntVarP(&parallel, "parallel", "j", 0, "number of parallel jobs")
	return cmd
}

func retryCmd(a *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "retry-failed",
		Short: "Retry workspaces that failed at the push step",
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
			fmt.Printf("retry-ready workspaces: %d\n", len(workspaces))
			for _, w := range workspaces {
				fmt.Printf("  ⚠ %s\n", w.Name)
			}
			return nil
		},
	}
}

func resumeCmd(a *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "resume-failed",
		Short: "Resume workspaces stuck at compress/convert",
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
			for _, w := range incomplete {
				fmt.Printf("  ⚠ %s — stuck at %s\n", w.Name, w.Stage)
			}
			return nil
		},
	}
}


