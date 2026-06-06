package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/appconfig"
	"github.com/chamrong/ivideo-hls/internal/pipeline"
)

func newResumeFailedCommand() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "resume-failed",
		Short: "Restart incomplete workspaces (stuck in compress/convert) from the source .mp4",
		Long: "Finds hero_<name>/ workspaces that did not reach the push stage and\n" +
			"weren't handled by retry-failed. Deletes the partial workspace (and,\n" +
			"by default, any _compressed.mp4 sibling) and drives the full pipeline\n" +
			"again from the original source video.\n\n" +
			"When resume_reuse_compressed is enabled in settings, a clean\n" +
			"_compressed.mp4 (no .partial sibling, valid duration) is kept and\n" +
			"used as the convert input — skipping the compress stage for that\n" +
			"video. See docs/PROCESS.md for the integrity rules.\n\n" +
			"Workspaces whose source .mp4 is missing are listed but skipped —\n" +
			"there's no safe way to resume without the source.",
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
			candidates, err := pipeline.FindIncompleteWorkspaces(cmd.Context(), cfg.ScriptDir)
			if err != nil {
				return fmt.Errorf("scan incomplete workspaces: %w", err)
			}
			if len(candidates) == 0 {
				fmt.Println(styleOk.Render("✔ no incomplete workspaces — nothing to resume"))
				return nil
			}
			resumable := selectResumable(candidates)
			plan := planResume(cmd.Context(), cfg, resumable)
			printResumePlan(candidates, plan)
			if len(resumable) == 0 {
				fmt.Println(styleErr.Render("✗ no candidates have a source .mp4 available — nothing to do"))
				return nil
			}
			if !assumeYes && !confirm("Proceed with this plan?") {
				fmt.Println(styleDim("cancelled"))
				return nil
			}
			return runResume(cmd.Context(), cfg, plan)
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

var styleResumeRow = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))

// resumePlan groups candidates by what the pipeline will actually do. The
// split exists because "reuse compressed" requires a different runner
// configuration (PreCompress=false, compressed file as source) from a
// fresh-compress run.
type resumePlan struct {
	reuseCompressed []pipeline.IncompleteWorkspace
	freshEncode     []pipeline.IncompleteWorkspace
}

// planResume splits resumable candidates into reuse-compressed vs fresh
// groups, honoring cfg.ResumeReuseCompressed and CompressedReusable.
func planResume(ctx context.Context, cfg *pipeline.Config, resumable []pipeline.IncompleteWorkspace) resumePlan {
	var plan resumePlan
	for _, c := range resumable {
		if cfg.ResumeReuseCompressed &&
			c.CompressedPath != "" &&
			pipeline.CompressedReusable(ctx, c.CompressedPath) {
			plan.reuseCompressed = append(plan.reuseCompressed, c)
			continue
		}
		plan.freshEncode = append(plan.freshEncode, c)
	}
	return plan
}

func printResumePlan(all []pipeline.IncompleteWorkspace, plan resumePlan) {
	fmt.Printf("Found %d incomplete workspace%s:\n\n",
		len(all), pluralS(len(all)))
	for _, c := range all {
		fmt.Printf("  %s  %s  %s\n",
			styleResumeRow.Render("⚠ "+c.Name),
			styleDim("stuck at "+string(c.Stage)+" — "+c.Hint),
			resumeSourceLine(c, plan))
	}
	fmt.Println()
	switch {
	case len(plan.reuseCompressed) > 0 && len(plan.freshEncode) > 0:
		fmt.Println(styleDim(fmt.Sprintf(
			"Plan: %d will skip compress (reuse existing _compressed.mp4); %d will re-encode from source.",
			len(plan.reuseCompressed), len(plan.freshEncode))))
	case len(plan.reuseCompressed) > 0:
		fmt.Println(styleDim(fmt.Sprintf(
			"Plan: all %d will skip compress and reuse the existing _compressed.mp4.",
			len(plan.reuseCompressed))))
	default:
		fmt.Println(styleDim("Plan: delete workspaces + compressed temps, re-run pipeline from source."))
	}
	fmt.Println()
}

func resumeSourceLine(c pipeline.IncompleteWorkspace, plan resumePlan) string {
	if !c.SourceExists {
		return styleErr.Render("✗ " + filepath.Base(c.SourcePath) + " (missing)")
	}
	for _, r := range plan.reuseCompressed {
		if r.Name == c.Name {
			return styleOk.Render("✓ reuse " + filepath.Base(c.CompressedPath))
		}
	}
	return styleOk.Render("✓ " + filepath.Base(c.SourcePath))
}

func selectResumable(all []pipeline.IncompleteWorkspace) []pipeline.IncompleteWorkspace {
	out := make([]pipeline.IncompleteWorkspace, 0, len(all))
	for _, c := range all {
		if c.SourceExists {
			out = append(out, c)
		}
	}
	return out
}

func runResume(ctx context.Context, cfg *pipeline.Config, plan resumePlan) error {
	totalOK, totalFail := 0, 0
	if len(plan.reuseCompressed) > 0 {
		ok, fail, err := runReuseCompressedPass(ctx, cfg, plan.reuseCompressed)
		if err != nil {
			return err
		}
		totalOK += ok
		totalFail += fail
	}
	if len(plan.freshEncode) > 0 {
		ok, fail, err := runFreshEncodePass(ctx, cfg, plan.freshEncode)
		if err != nil {
			return err
		}
		totalOK += ok
		totalFail += fail
	}
	if totalFail > 0 {
		fmt.Println(styleErr.Render(fmt.Sprintf("✗ %d ok · %d failed", totalOK, totalFail)))
		os.Exit(1)
	}
	fmt.Println(styleOk.Render(fmt.Sprintf("✔ resumed %d workspace%s", totalOK, pluralS(totalOK))))
	return nil
}

// runReuseCompressedPass reruns the pipeline with PreCompress disabled and
// the pre-existing _compressed.mp4 fed in as the source. Only the workspace
// is deleted; the compressed file is preserved.
func runReuseCompressedPass(ctx context.Context, cfg *pipeline.Config, candidates []pipeline.IncompleteWorkspace) (int, int, error) {
	for _, c := range candidates {
		if err := cleanWorkspaceOnly(c); err != nil {
			return 0, 0, err
		}
		fmt.Println(styleDim("  reuse: keeping " + filepath.Base(c.CompressedPath) + " (skipping compress)"))
	}
	reuseCfg := *cfg
	reuseCfg.PreCompress = false
	reuseCfg.Videos = compressedPathsOf(candidates)
	runner := pipeline.NewRunner(&reuseCfg, plainLogEmitter())
	results := runner.Run(ctx)
	ok, fail := pipeline.Summary(results)
	return ok, fail, nil
}

// runFreshEncodePass reruns the pipeline from scratch for the candidates
// that couldn't (or shouldn't) skip compress.
func runFreshEncodePass(ctx context.Context, cfg *pipeline.Config, candidates []pipeline.IncompleteWorkspace) (int, int, error) {
	for _, c := range candidates {
		if err := cleanPartialArtifacts(c); err != nil {
			return 0, 0, err
		}
	}
	freshCfg := *cfg
	freshCfg.Videos = sourcesOf(candidates)
	runner := pipeline.NewRunner(&freshCfg, plainLogEmitter())
	results := runner.Run(ctx)
	ok, fail := pipeline.Summary(results)
	return ok, fail, nil
}

// cleanWorkspaceOnly removes the hero_* directory but leaves the compressed
// sibling in place. Used by the reuse-compressed path.
func cleanWorkspaceOnly(c pipeline.IncompleteWorkspace) error {
	if err := os.RemoveAll(c.Workspace); err != nil {
		return fmt.Errorf("remove workspace %s: %w", c.Workspace, err)
	}
	fmt.Println(styleDim("  cleaned: " + filepath.Base(c.Workspace)))
	return nil
}

// cleanPartialArtifacts removes the workspace AND any compressed sibling —
// the old, strict policy used when reuse is off (or the sibling isn't
// trustworthy).
func cleanPartialArtifacts(c pipeline.IncompleteWorkspace) error {
	if err := os.RemoveAll(c.Workspace); err != nil {
		return fmt.Errorf("remove workspace %s: %w", c.Workspace, err)
	}
	if c.CompressedPath != "" {
		if err := os.Remove(c.CompressedPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", c.CompressedPath, err)
		}
	}
	fmt.Println(styleDim("  cleaned: " + filepath.Base(c.Workspace) +
		conditional(c.CompressedPath != "", " + "+filepath.Base(c.CompressedPath), "")))
	return nil
}

func sourcesOf(candidates []pipeline.IncompleteWorkspace) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.SourcePath)
	}
	return out
}

func compressedPathsOf(candidates []pipeline.IncompleteWorkspace) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.CompressedPath)
	}
	return out
}

func conditional(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
