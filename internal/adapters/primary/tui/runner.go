package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/job"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

// Messages
type eventMsg job.Event
type finishedMsg struct{ results []job.Result }
type slotTickMsg struct{}

const slotTickInterval = 500 * time.Millisecond

func tickSlots() tea.Cmd {
	return tea.Tick(slotTickInterval, func(time.Time) tea.Msg { return slotTickMsg{} })
}

type jobState struct {
	name      string
	stage     job.Stage
	percent   float64
	done      bool
	failed    bool
	lastMsg   string
	failedMsg string
	startedAt time.Time
	endedAt   time.Time
	speed     float64
	bitrate   string
}

func (j *jobState) duration() time.Duration {
	if j.startedAt.IsZero() || j.endedAt.IsZero() {
		return 0
	}
	return j.endedAt.Sub(j.startedAt)
}

// RunModel is a bubbletea model that drives *app.Runner with a live dashboard.
type RunModel struct {
	a       *app.App
	cfg     settings.Settings
	videos  []video.Video
	events    chan job.Event
	resultsCh chan []job.Result
	ctx       context.Context
	cancel    context.CancelFunc
	results   []job.Result
	done      bool

	spinner spinner.Model
	prog    progress.Model

	jobs     map[string]*jobState
	jobOrder []string
	logs     []job.Event
	width    int
	height   int
	started  time.Time
}

// NewRunModel creates a run-dashboard model wired to *app.App.
func NewRunModel(a *app.App, cfg settings.Settings, videos []video.Video) *RunModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	p := progress.New(progress.WithScaledGradient("#7C3AED", "#22D3EE"))
	p.Width = 40
	p.ShowPercentage = true

	return &RunModel{
		a:         a,
		cfg:       cfg,
		videos:    videos,
		events:    make(chan job.Event, 1024),
		resultsCh: make(chan []job.Result, 1),
		spinner:   sp,
		prog:      p,
		jobs:      map[string]*jobState{},
		started:   time.Now(),
	}
}

func (m *RunModel) emitter() job.Emitter {
	return job.FuncEmitter(func(ev job.Event) {
		select {
		case m.events <- ev:
		default:
		}
	})
}

func (m *RunModel) Init() tea.Cmd {
	m.ctx, m.cancel = context.WithCancel(context.Background())
	go func() {
		results := m.a.Runner.Run(m.ctx, m.videos, m.cfg, m.emitter())
		m.resultsCh <- results
		close(m.events)
	}()
	return tea.Batch(m.spinner.Tick, waitForEvent(m.events, m.resultsCh), tickSlots())
}

func (m *RunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.prog.Width = clampInt(m.width-30, 20, 60)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "q":
			if m.done {
				return m, tea.Quit
			}
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case eventMsg:
		m.ingest(job.Event(msg))
		return m, waitForEvent(m.events, m.resultsCh)
	case slotTickMsg:
		if m.done {
			return m, nil
		}
		return m, tickSlots()
	case finishedMsg:
		m.results = msg.results
		m.done = true
	}
	return m, nil
}

func (m *RunModel) ingest(ev job.Event) {
	if ev.Job == "" || ev.Job == job.BaseJob {
		m.logs = append(m.logs, ev)
		m.trimLogs()
		return
	}
	js, ok := m.jobs[ev.Job]
	if !ok {
		js = &jobState{name: ev.Job, startedAt: ev.Time}
		m.jobs[ev.Job] = js
		m.jobOrder = append(m.jobOrder, ev.Job)
	}
	js.stage = ev.Stage
	if ev.Message != "" {
		js.lastMsg = ev.Message
	}
	if ev.Percent > 0 {
		js.percent = stagePercent(ev.Stage, ev.Percent)
	} else {
		js.percent = stageProgress(ev.Stage)
	}
	if ev.Speed > 0 {
		js.speed = ev.Speed
	}
	if ev.Bitrate != "" {
		js.bitrate = ev.Bitrate
	}
	switch ev.Stage {
	case job.StageDone:
		js.done = true
		js.percent = 1.0
		js.endedAt = ev.Time
	case job.StageFailed:
		js.failed = true
		js.failedMsg = ev.Message
		js.endedAt = ev.Time
	}
	if !isProgressTick(ev) {
		m.logs = append(m.logs, ev)
		m.trimLogs()
	}
}

func isProgressTick(ev job.Event) bool {
	if ev.Percent <= 0 {
		return false
	}
	if ev.Stage != job.StageCompress && ev.Stage != job.StageConvert {
		return false
	}
	return ev.Level == job.LevelDim
}

func stagePercent(stage job.Stage, local float64) float64 {
	start, span := stageRange(stage)
	return start + span*local
}

func stageRange(stage job.Stage) (start, span float64) {
	switch stage {
	case job.StageQueued:
		return 0.00, 0.05
	case job.StageWorkspace:
		return 0.05, 0.10
	case job.StageCompress:
		return 0.15, 0.20
	case job.StageConvert:
		return 0.35, 0.45
	case job.StageRename:
		return 0.80, 0.05
	case job.StageGitPush:
		return 0.85, 0.15
	case job.StageDone, job.StageFailed:
		return 1.0, 0
	}
	return 0, 0
}

func (m *RunModel) trimLogs() {
	const maxLogs = 200
	if len(m.logs) > maxLogs {
		m.logs = m.logs[len(m.logs)-maxLogs:]
	}
}

func stageProgress(s job.Stage) float64 {
	start, span := stageRange(s)
	return start + span/2
}

func waitForEvent(ch <-chan job.Event, results <-chan []job.Result) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return finishedMsg{results: <-results}
		}
		return eventMsg(ev)
	}
}

func (m *RunModel) View() string {
	width := m.frameWidth()
	sections := []string{
		m.renderHeader(),
		rule(width),
		m.renderRunning(width),
	}
	if done := m.renderDone(width); done != "" {
		sections = append(sections, rule(width), styleAccent.Render("Done"), done)
	}
	if fails := m.renderFailures(); fails != "" {
		sections = append(sections, rule(width), fails)
	}
	sections = append(sections, rule(width), m.renderLogs(width))
	sections = append(sections, rule(width), m.renderFooter())
	return strings.Join(sections, "\n")
}

func (m *RunModel) frameWidth() int {
	return clampInt(m.width, 80, 140)
}

func rule(width int) string {
	return styleDim.Render(strings.Repeat("─", width))
}

func (m *RunModel) renderFailures() string {
	var lines []string
	for _, name := range m.jobOrder {
		js := m.jobs[name]
		if !js.failed {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s",
			styleError.Render("✗"),
			styleInfo.Render(name),
			styleDim.Render(js.failedMsg),
		))
	}
	if len(lines) == 0 {
		return ""
	}
	return styleError.Render("⚠ Failures") + "\n" + strings.Join(lines, "\n")
}

type jobCounts struct {
	done, failed, running, total int
}

func (c jobCounts) completed() int { return c.done + c.failed }
func (c jobCounts) remaining() int { return c.total - c.completed() }

func (m *RunModel) countJobs() jobCounts {
	c := jobCounts{total: len(m.videos)}
	for _, js := range m.jobs {
		switch {
		case js.failed:
			c.failed++
		case js.done:
			c.done++
		default:
			c.running++
		}
	}
	return c
}

func (m *RunModel) renderHeader() string {
	title := styleTitle.Render(" ivideo-hls ")
	counts := m.countJobs()
	elapsed := time.Since(m.started).Round(time.Second)
	counter := styleInfo.Render(fmt.Sprintf("%d/%d", counts.completed(), counts.total))
	status := m.runStatus(counts)

	segments := []string{title, status, counter + " " + styleDim.Render("done"), styleDim.Render(elapsed.String())}
	if eta := m.estimateETA(counts); eta != "" && !m.done {
		segments = append(segments, styleDim.Render("ETA ~"+eta))
	}
	if m.cfg.RemoteURL != "" {
		segments = append(segments, styleDim.Render("→ "+m.cfg.RemoteURL))
	}
	return strings.Join(segments, styleDim.Render(" · "))
}

func (m *RunModel) runStatus(c jobCounts) string {
	if !m.done {
		return m.spinner.View() + " " + styleAccent.Render("processing")
	}
	if c.failed == 0 {
		return styleSuccess.Render(fmt.Sprintf("✔ all %d job%s complete", c.done, plural(c.done)))
	}
	return styleError.Render(fmt.Sprintf("✗ %d ok · %d failed", c.done, c.failed))
}

func (m *RunModel) estimateETA(c jobCounts) string {
	if c.remaining() <= 0 || c.completed() == 0 {
		return ""
	}
	avg := m.averageCompletedDuration()
	if avg <= 0 {
		return ""
	}
	parallel := max(m.cfg.MaxParallel, 1)
	pending := max(c.remaining()-c.running, 0)
	waves := (pending + parallel - 1) / parallel
	estimate := avg*time.Duration(waves) + avg/2
	return estimate.Round(time.Second).String()
}

func (m *RunModel) averageCompletedDuration() time.Duration {
	var total time.Duration
	var n int
	for _, js := range m.jobs {
		d := js.duration()
		if d > 0 {
			total += d
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return total / time.Duration(n)
}

func (m *RunModel) renderRunning(width int) string {
	running := m.filterJobs(func(js *jobState) bool { return !js.done && !js.failed })
	if len(running) == 0 {
		if m.done {
			return styleDim.Render("  (no active jobs)")
		}
		return styleDim.Render("  waiting for first job…")
	}
	var rows []string
	for _, js := range running {
		rows = append(rows, m.renderRunningRow(js, width))
	}
	return strings.Join(rows, "\n")
}

func (m *RunModel) renderRunningRow(js *jobState, width int) string {
	const (
		barWidth   = 28
		nameWidth  = 24
		stageWidth = 10
	)
	m.prog.Width = barWidth
	bar := m.prog.ViewAs(js.percent)
	pct := fmt.Sprintf("%3.0f%%", js.percent*100)
	name := lipgloss.NewStyle().Width(nameWidth).Render(truncate(js.name, nameWidth-1))
	stage := renderStageBadge(js)
	speed := styleDim.Render(formatSpeed(js.speed))
	bitrate := styleDim.Render(formatBitrate(js.bitrate))
	_ = width
	return fmt.Sprintf(" %s %s  %s  %s  %s  %s",
		bar, styleAccent.Render(pct), styleInfo.Render(name), stage, speed, bitrate)
}

func (m *RunModel) renderDone(width int) string {
	const nameWidth = 24
	done := m.filterJobs(func(js *jobState) bool { return js.done && !js.failed })
	if len(done) == 0 {
		return ""
	}
	var rows []string
	for _, js := range done {
		name := lipgloss.NewStyle().Width(nameWidth).Render(truncate(js.name, nameWidth-1))
		dur := js.duration().Round(time.Second)
		line := fmt.Sprintf(" %s %s  %s  %s",
			styleSuccess.Render("✓"),
			styleInfo.Render(name),
			styleDim.Render(dur.String()),
			styleDim.Render(truncate(js.lastMsg, width-nameWidth-20)))
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m *RunModel) filterJobs(keep func(*jobState) bool) []*jobState {
	out := make([]*jobState, 0, len(m.jobOrder))
	for _, name := range m.jobOrder {
		js := m.jobs[name]
		if keep(js) {
			out = append(out, js)
		}
	}
	return out
}

func formatSpeed(s float64) string {
	if s <= 0 {
		return "      "
	}
	return fmt.Sprintf("%4.1fx", s)
}

func formatBitrate(b string) string {
	if b == "" || b == "N/A" {
		return ""
	}
	b = strings.TrimSuffix(b, "bits/s")
	b = strings.TrimSuffix(b, "its/s")
	return b
}

func renderStageBadge(js *jobState) string {
	text := string(js.stage)
	if text == "" {
		text = "pending"
	}
	style := lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	switch {
	case js.failed:
		style = style.Background(colError)
		text = "failed"
	case js.done:
		style = style.Background(colSuccess)
		text = "done"
	case js.stage == job.StageCompress || js.stage == job.StageConvert:
		style = style.Background(colPrimary)
	case js.stage == job.StageGitPush:
		style = style.Background(colAccent).Foreground(lipgloss.Color("#000000"))
	case js.stage == job.StageWorkspace || js.stage == job.StageQueued:
		style = style.Background(colDim)
	default:
		style = style.Background(colDim)
	}
	return style.Render(fmt.Sprintf("%-10s", text))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

func (m *RunModel) renderLogs(width int) string {
	title := styleAccent.Render("Log · tail")
	if len(m.logs) == 0 {
		return title + "\n" + styleDim.Render("  …")
	}
	visible := m.logLineCount()
	start := max(len(m.logs)-visible, 0)
	lines := make([]string, 0, visible)
	for _, ev := range m.logs[start:] {
		lines = append(lines, renderLogLine(ev))
	}
	_ = width
	return title + "\n" + strings.Join(lines, "\n")
}

func (m *RunModel) logLineCount() int {
	if m.height <= 20 {
		return 3
	}
	if m.height <= 30 {
		return 6
	}
	return 10
}

func (m *RunModel) renderFooter() string {
	if m.done {
		counts := m.countJobs()
		if counts.failed > 0 {
			return styleHelp.Render(" press q to exit") + "\n" +
				styleWarn.Render(" ⚑  run `ivideo-hls recover` to push or re-encode failed videos")
		}
		return styleHelp.Render(" press q to exit")
	}
	return styleHelp.Render(" ctrl+c cancel  ·  q quit (after done)")
}

func renderLogLine(ev job.Event) string {
	ts := styleDim.Render(ev.Time.Format("15:04:05"))
	jobStr := ""
	if ev.Job != "" {
		jobStr = styleAccent.Render("[" + ev.Job + "]")
	}
	msg := ev.Message
	var styled string
	switch ev.Level {
	case job.LevelSuccess:
		styled = styleSuccess.Render("✓ " + msg)
	case job.LevelWarn:
		styled = styleWarn.Render("! " + msg)
	case job.LevelError:
		styled = styleError.Render("✗ " + msg)
	case job.LevelDim:
		styled = styleDim.Render(iconDot + " " + msg)
	default:
		styled = styleInfo.Render(msg)
	}
	return ts + " " + jobStr + " " + styled
}

// RunTUI runs the full pipeline with a live progress dashboard.
// It returns the job results when the run completes.
func RunTUI(a *app.App, cfg settings.Settings, videos []video.Video) ([]job.Result, error) {
	m := NewRunModel(a, cfg, videos)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return nil, err
	}
	return m.results, nil
}
