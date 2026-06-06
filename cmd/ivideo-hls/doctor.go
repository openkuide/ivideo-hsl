package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/doctor"
)

var (
	doctorOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
	doctorWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	doctorFail = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	doctorDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	doctorHead = lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE")).Bold(true)
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local setup (ffmpeg, git, config, remote reachability)",
		Long: "Runs read-only checks against the local environment and your persisted\n" +
			"ivideo-hls config. Reports each as OK, warn, or fail with a remediation\n" +
			"hint. Exits 1 if any check fails; warnings do not fail the exit code.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := doctor.Check(cmd.Context())
			printDoctorResult(result)
			if !result.OK() {
				os.Exit(1)
			}
			return nil
		},
	}
}

func printDoctorResult(r doctor.Result) {
	fmt.Println(doctorHead.Render("ivideo-hls · doctor"))
	fmt.Println()
	labelWidth := longestTitle(r.Findings) + 2
	titleStyle := lipgloss.NewStyle().Width(labelWidth)
	for _, f := range r.Findings {
		fmt.Printf(" %s  %s  %s\n",
			levelBadge(f.Level),
			titleStyle.Render(f.Title),
			f.Detail)
		if f.Hint != "" && f.Level != doctor.LevelOK {
			fmt.Printf("     %s  %s\n",
				lipgloss.NewStyle().Width(labelWidth).Render(""),
				doctorDim.Render("↳ "+f.Hint))
		}
	}
	fmt.Println()
	fmt.Println(summary(r))
}

func levelBadge(l doctor.Level) string {
	switch l {
	case doctor.LevelOK:
		return doctorOK.Render("✓")
	case doctor.LevelWarn:
		return doctorWarn.Render("!")
	}
	return doctorFail.Render("✗")
}

func longestTitle(findings []doctor.Finding) int {
	n := 0
	for _, f := range findings {
		if len(f.Title) > n {
			n = len(f.Title)
		}
	}
	return n
}

func summary(r doctor.Result) string {
	var ok, warn, fail int
	for _, f := range r.Findings {
		switch f.Level {
		case doctor.LevelOK:
			ok++
		case doctor.LevelWarn:
			warn++
		case doctor.LevelFail:
			fail++
		}
	}
	switch {
	case fail > 0:
		return doctorFail.Render(fmt.Sprintf("✗ %d failure(s), %d warning(s), %d ok", fail, warn, ok))
	case warn > 0:
		return doctorWarn.Render(fmt.Sprintf("! %d warning(s), %d ok", warn, ok))
	}
	return doctorOK.Render(fmt.Sprintf("✔ all %d checks passed", ok))
}
