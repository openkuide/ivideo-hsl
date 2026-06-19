package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type settingsField int

const (
	fieldURL settingsField = iota
	fieldAuthMethod
	fieldToken
	fieldPublicURL
	fieldSourceDir
	fieldRecursive
	fieldParallel
	fieldQuality
	fieldCompression
	fieldPreCompress
	fieldKeepSource
	fieldPush
	fieldCleanup
	fieldReuseCompressed
	fieldCount
)

type tokenSource int

const (
	tokenFromConfig tokenSource = iota
	tokenFromEnv
	tokenNotSet
)

type testResult struct {
	ok       bool
	message  string
	duration time.Duration
}

type testResultMsg testResult

// SettingsModel is the persistent-config editor wired to *app.App.
type SettingsModel struct {
	initial    settings.Settings
	current    settings.Settings
	envToken   string
	a          *app.App
	cursor     settingsField
	width      int
	height     int
	editing    bool
	buffer     string
	confirming bool
	saveMsg    string
	errorMsg   string
	testing    bool
	testResult *testResult
}

// Persisted returns the last-persisted settings state.
func (m *SettingsModel) Persisted() settings.Settings { return m.initial }

// NewSettingsModel creates the settings editor backed by *app.App.
func NewSettingsModel(a *app.App, loaded settings.Settings, envToken string) *SettingsModel {
	return &SettingsModel{
		initial:  loaded,
		current:  loaded,
		envToken: envToken,
		a:        a,
	}
}

func (m *SettingsModel) Init() tea.Cmd { return nil }

func (m *SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case testResultMsg:
		r := testResult(msg)
		m.testResult = &r
		m.testing = false
	case tea.KeyMsg:
		switch {
		case m.confirming:
			return m.updateConfirming(msg)
		case m.editing:
			return m.updateEditing(msg)
		default:
			return m.updateNavigating(msg)
		}
	}
	return m, nil
}

func (m *SettingsModel) updateConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s":
		m.save()
		if m.errorMsg != "" {
			return m, nil
		}
		return m, tea.Quit
	case "d":
		m.current = m.initial
		return m, tea.Quit
	case "esc":
		m.confirming = false
	}
	return m, nil
}

func (m *SettingsModel) updateNavigating(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		if m.dirty() {
			m.confirming = true
			return m, nil
		}
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < fieldCount-1 {
			m.cursor++
		}
	case "enter":
		m.enterEditOrToggle()
	case " ":
		m.toggleBoolField()
	case "left", "h":
		m.adjustField(-1)
	case "right", "l":
		m.adjustField(1)
	case "s":
		m.save()
	case "d":
		m.resetDefaults()
	case "t":
		return m, m.runConnectionTest()
	}
	return m, nil
}

func (m *SettingsModel) updateEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = false
		m.buffer = ""
	case "enter":
		m.commitEdit()
	case "backspace":
		if n := len(m.buffer); n > 0 {
			m.buffer = m.buffer[:n-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.buffer += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *SettingsModel) enterEditOrToggle() {
	switch m.cursor {
	case fieldURL:
		m.editing = true
		m.buffer = m.current.RemoteURL
	case fieldToken:
		m.editing = true
		m.buffer = m.current.Token
	case fieldPublicURL:
		m.editing = true
		m.buffer = m.current.PublicURLPattern
	case fieldSourceDir:
		m.editing = true
		m.buffer = m.current.SourceDir
	case fieldAuthMethod:
		m.toggleAuth()
	case fieldParallel, fieldQuality, fieldCompression:
		m.adjustField(1)
	case fieldPreCompress, fieldKeepSource, fieldPush, fieldCleanup, fieldRecursive, fieldReuseCompressed:
		m.toggleBoolField()
	}
}

func (m *SettingsModel) commitEdit() {
	switch m.cursor {
	case fieldURL:
		m.current.RemoteURL = strings.TrimSpace(m.buffer)
		m.current.AuthMethod = inferAuthMethod(m.current.RemoteURL, m.current.AuthMethod)
	case fieldToken:
		m.current.Token = strings.TrimSpace(m.buffer)
	case fieldPublicURL:
		m.current.PublicURLPattern = strings.TrimSpace(m.buffer)
	case fieldSourceDir:
		m.current.SourceDir = strings.TrimSpace(m.buffer)
	}
	m.editing = false
	m.buffer = ""
}

func (m *SettingsModel) toggleAuth() {
	if m.current.AuthMethod == settings.AuthHTTPS {
		m.current.AuthMethod = settings.AuthSSH
		return
	}
	m.current.AuthMethod = settings.AuthHTTPS
}

func (m *SettingsModel) toggleBoolField() {
	switch m.cursor {
	case fieldPreCompress:
		m.current.PreCompress = !m.current.PreCompress
	case fieldKeepSource:
		m.current.KeepSource = !m.current.KeepSource
	case fieldPush:
		m.current.Push = !m.current.Push
	case fieldCleanup:
		m.current.Cleanup = !m.current.Cleanup
	case fieldRecursive:
		m.current.Recursive = !m.current.Recursive
	case fieldReuseCompressed:
		m.current.ResumeReuseCompressed = !m.current.ResumeReuseCompressed
	case fieldAuthMethod:
		m.toggleAuth()
	}
}

func (m *SettingsModel) adjustField(delta int) {
	switch m.cursor {
	case fieldParallel:
		m.current.MaxParallel = clampInt(m.current.MaxParallel+delta, 0, 32)
	case fieldQuality:
		m.current.Quality = stepQuality(m.current.Quality, delta)
	case fieldCompression:
		m.current.Compression = stepCompression(m.current.Compression, delta)
	case fieldAuthMethod:
		target := settings.AuthSSH
		if delta > 0 {
			target = settings.AuthHTTPS
		}
		m.current.AuthMethod = target
	}
}

func stepQuality(current video.Quality, delta int) video.Quality {
	choices := []video.Quality{video.QualityLow, video.QualityMedium, video.QualityHigh}
	idx := 1 // default medium
	for i, c := range choices {
		if c == current {
			idx = i
			break
		}
	}
	idx = clampInt(idx+delta, 0, len(choices)-1)
	return choices[idx]
}

func stepCompression(current video.Compression, delta int) video.Compression {
	choices := []video.Compression{video.CompressionFast, video.CompressionBalanced, video.CompressionBest}
	idx := 1 // default balanced
	for i, c := range choices {
		if c == current {
			idx = i
			break
		}
	}
	idx = clampInt(idx+delta, 0, len(choices)-1)
	return choices[idx]
}

func (m *SettingsModel) save() {
	if err := validateRemoteURL(m.current.RemoteURL); err != nil {
		m.errorMsg = err.Error()
		m.saveMsg = ""
		return
	}
	if m.a != nil {
		if err := m.a.Config.Save(m.current); err != nil {
			m.errorMsg = err.Error()
			m.saveMsg = ""
			return
		}
	}
	m.initial = m.current
	m.saveMsg = "saved"
	m.errorMsg = ""
}

func (m *SettingsModel) resetDefaults() {
	m.current = settings.Settings{
		RemoteURL:   "git@github.com:username/repo.git",
		AuthMethod:  settings.AuthSSH,
		Quality:     video.QualityMedium,
		Compression: video.CompressionBalanced,
	}
	m.errorMsg = ""
	m.saveMsg = ""
}

func (m *SettingsModel) runConnectionTest() tea.Cmd {
	if m.testing {
		return nil
	}
	m.testing = true
	m.testResult = nil
	token := m.effectiveToken()
	url := effectiveRemoteURL(m.current.RemoteURL, token, m.current.AuthMethod)
	return func() tea.Msg {
		return testResultMsg(testRemote(url))
	}
}

func (m *SettingsModel) effectiveToken() string {
	if m.current.Token != "" {
		return m.current.Token
	}
	return m.envToken
}

func testRemote(url string) testResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", "ls-remote", url)
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)
	if err != nil {
		return testResult{ok: false, message: firstLine(string(out), err.Error()), duration: dur}
	}
	return testResult{ok: true, message: "reachable", duration: dur}
}

func firstLine(out, fallback string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return fallback
	}
	if idx := strings.IndexByte(out, '\n'); idx > 0 {
		return out[:idx]
	}
	return out
}

// inferAuthMethod picks the auth method from the URL scheme.
func inferAuthMethod(url string, current settings.AuthMethod) settings.AuthMethod {
	switch {
	case strings.HasPrefix(url, "https://"):
		return settings.AuthHTTPS
	case strings.HasPrefix(url, "git@"), strings.HasPrefix(url, "ssh://"):
		return settings.AuthSSH
	}
	return current
}

// effectiveRemoteURL injects a token into HTTPS URLs.
func effectiveRemoteURL(displayURL, token string, method settings.AuthMethod) string {
	if method != settings.AuthHTTPS || token == "" {
		return displayURL
	}
	if !strings.HasPrefix(displayURL, "https://") {
		return displayURL
	}
	if strings.Contains(displayURL[len("https://"):], "@") {
		return displayURL
	}
	return "https://" + token + "@" + strings.TrimPrefix(displayURL, "https://")
}

// validateRemoteURL validates the remote URL.
func validateRemoteURL(url string) error {
	switch {
	case url == "":
		return fmt.Errorf("remote URL is required")
	case strings.HasPrefix(url, "git@"),
		strings.HasPrefix(url, "ssh://"),
		strings.HasPrefix(url, "https://"):
		return nil
	}
	return fmt.Errorf("URL must start with git@, ssh://, or https://")
}

// maskToken returns a short, log-safe preview.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return strings.Repeat("•", len(token))
	}
	return strings.Repeat("•", 8) + token[len(token)-4:]
}

func (m *SettingsModel) View() string {
	header := styleTitle.Render(" ivideo-hls · settings ") + styleSubtitle.Render("  persistent config "+iconSpark)

	rows := []string{
		m.renderGroupTitle("Remote"),
		m.renderRow(fieldURL, "Push URL", m.renderURLValue()),
		m.renderRow(fieldAuthMethod, "Auth method", m.renderAuthValue()),
		m.renderRow(fieldToken, "Token", m.renderTokenValue()),
		m.renderRow(fieldPublicURL, "Playback URL", m.renderPublicURLValue()),
		"",
		m.renderGroupTitle("Source"),
		m.renderRow(fieldSourceDir, "Default folder", m.renderSourceDirValue()),
		m.renderRow(fieldRecursive, "Recursive scan", boolLabel(m.current.Recursive)),
		"",
		m.renderGroupTitle("Defaults"),
		m.renderRow(fieldParallel, "Parallel jobs", m.renderParallelValue()),
		m.renderRow(fieldQuality, "Quality", string(m.current.Quality)),
		m.renderRow(fieldCompression, "Compression", string(m.current.Compression)),
		m.renderRow(fieldPreCompress, "Pre-compress", boolLabel(m.current.PreCompress)),
		m.renderRow(fieldKeepSource, "Keep source .mp4", boolLabel(m.current.KeepSource)),
		m.renderRow(fieldPush, "Push to remote", boolLabel(m.current.Push)),
		m.renderRow(fieldCleanup, "Clean up workspace", boolLabel(m.current.Cleanup)),
		m.renderRow(fieldReuseCompressed, "Reuse compressed on resume", boolLabel(m.current.ResumeReuseCompressed)),
	}

	hint := styleMuted.Render("ℹ " + settingsFieldHint(m.cursor))
	status := m.renderStatus()

	panel := stylePanel.Width(m.panelWidth()).Render(
		strings.Join(rows, "\n") + "\n\n" + hint + status,
	)

	return header + "\n" + panel + "\n" + styleHelp.Render(m.helpText())
}

func (m *SettingsModel) renderGroupTitle(label string) string {
	return styleAccent.Render(label)
}

func (m *SettingsModel) renderRow(field settingsField, label, value string) string {
	active := m.cursor == field
	marker := "  "
	labelStyle := styleMuted
	valueStyle := styleInfo
	if active {
		marker = styleAccent.Render(iconArrow + " ")
		valueStyle = styleAccent
	}
	labelStr := lipgloss.NewStyle().Width(18).Render(label)
	return marker + labelStyle.Render(labelStr) + "  " + valueStyle.Render(value)
}

func (m *SettingsModel) renderURLValue() string {
	if m.editing && m.cursor == fieldURL {
		return m.buffer + styleAccent.Render("▌")
	}
	if m.current.RemoteURL == "" {
		return styleDim.Render("(not set)")
	}
	return m.current.RemoteURL
}

func (m *SettingsModel) renderAuthValue() string {
	ssh := "SSH"
	https := "HTTPS + token"
	if m.current.AuthMethod == settings.AuthHTTPS {
		return styleDim.Render(ssh) + "  /  " + styleAccent.Render(https)
	}
	return styleAccent.Render(ssh) + "  /  " + styleDim.Render(https)
}

func (m *SettingsModel) renderParallelValue() string {
	n := m.current.MaxParallel
	if n <= 1 {
		return styleDim.Render("serial (1)") + "  " + styleMuted.Render(fmt.Sprintf("· %d CPU cores available", runtime.NumCPU()))
	}
	return fmt.Sprintf("%d concurrent", n) + "  " + styleMuted.Render(fmt.Sprintf("· %d CPU cores available", runtime.NumCPU()))
}

func (m *SettingsModel) renderSourceDirValue() string {
	if m.editing && m.cursor == fieldSourceDir {
		return m.buffer + styleAccent.Render("▌")
	}
	if m.current.SourceDir == "" {
		return styleDim.Render("(current working directory)")
	}
	return m.current.SourceDir
}

func (m *SettingsModel) renderPublicURLValue() string {
	if m.editing && m.cursor == fieldPublicURL {
		return m.buffer + styleAccent.Render("▌")
	}
	if m.current.PublicURLPattern == "" {
		return styleDim.Render("(local path — no playback URL set)")
	}
	return m.current.PublicURLPattern
}

func (m *SettingsModel) renderTokenValue() string {
	if m.editing && m.cursor == fieldToken {
		return m.buffer + styleAccent.Render("▌")
	}
	token, source := m.resolvedToken()
	if token == "" {
		return styleDim.Render("(not set)")
	}
	return maskToken(token) + " " + styleDim.Render("("+sourceLabel(source)+")")
}

func (m *SettingsModel) resolvedToken() (string, tokenSource) {
	if m.current.Token != "" {
		return m.current.Token, tokenFromConfig
	}
	if m.envToken != "" {
		return m.envToken, tokenFromEnv
	}
	return "", tokenNotSet
}

func sourceLabel(s tokenSource) string {
	switch s {
	case tokenFromConfig:
		return "from config"
	case tokenFromEnv:
		return "from $IVIDEO_HLS_TOKEN"
	}
	return "not set"
}

func (m *SettingsModel) renderStatus() string {
	var parts []string
	if m.testing {
		parts = append(parts, styleDim.Render("testing connection…"))
	}
	if m.testResult != nil {
		parts = append(parts, formatTestResult(*m.testResult))
	}
	if m.saveMsg != "" {
		parts = append(parts, styleSuccess.Render("✓ "+m.saveMsg))
	}
	if m.errorMsg != "" {
		parts = append(parts, styleError.Render("✗ "+m.errorMsg))
	}
	if m.dirty() {
		parts = append(parts, styleWarn.Render("● unsaved changes"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n" + strings.Join(parts, "    ")
}

func formatTestResult(r testResult) string {
	if r.ok {
		return styleSuccess.Render(fmt.Sprintf("✓ %s (%s)", r.message, r.duration.Round(time.Millisecond)))
	}
	return styleError.Render("✗ " + r.message)
}

func (m *SettingsModel) dirty() bool {
	return m.current != m.initial
}

func (m *SettingsModel) panelWidth() int {
	return clampInt(m.width-2, 60, 110)
}

func (m *SettingsModel) helpText() string {
	switch {
	case m.confirming:
		return styleWarn.Render("● unsaved changes — ") +
			styleKey("s") + " save & exit  ·  " +
			styleKey("d") + " discard & exit  ·  " +
			styleKey("esc") + " keep editing"
	case m.editing:
		return "type to edit · enter save edit · esc cancel"
	}
	return "↑/↓ move · enter edit · ←/→ adjust · space toggle · s save · t test · d reset · esc back"
}

func settingsFieldHint(f settingsField) string {
	switch f {
	case fieldURL:
		return "Where `git push` sends commits. SSH (git@host:path) or HTTPS (https://host/path)."
	case fieldAuthMethod:
		return "SSH uses your key agent. HTTPS injects the token into the push URL."
	case fieldToken:
		return "GitHub PAT or equivalent. $IVIDEO_HLS_TOKEN overrides."
	case fieldPublicURL:
		return "HTTP(S) template for urls.txt. {branch}, {subdir}, {filename}."
	case fieldSourceDir:
		return "Folder scanned when no --source / -i / -a is given. Empty = cwd."
	case fieldRecursive:
		return "Walk subdirectories."
	case fieldParallel:
		return "Concurrent encodes."
	case fieldReuseCompressed:
		return "When ON, resume-failed skips compress if a clean _compressed.mp4 is present."
	case fieldPush:
		return "When OFF, commit locally but skip `git push`."
	case fieldCleanup:
		return "When OFF, keep hero_<name>/ workspace after success."
	case fieldQuality:
		return "Default output resolution/bitrate."
	case fieldCompression:
		return "Default encoder preset."
	case fieldPreCompress:
		return "When ON, runs a compression pass before HLS by default."
	case fieldKeepSource:
		return "When ON, original .mp4 is kept after success by default."
	}
	return ""
}

// RunSettings opens the settings editor and returns the last-persisted settings.
func RunSettings(a *app.App, loaded settings.Settings, envToken string) (settings.Settings, error) {
	m := NewSettingsModel(a, loaded, envToken)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return loaded, err
	}
	return m.Persisted(), nil
}
