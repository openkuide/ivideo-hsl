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

	"github.com/chamrong/ivideo-hls/internal/appconfig"
)

// settingsField identifies a row on the settings screen. Listed in display
// order so navigation is a simple integer cursor.
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

// tokenSource describes where the currently-displayed token came from so the
// user can tell apart "loaded from config" vs "loaded from $IVIDEO_HLS_TOKEN".
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

// SettingsModel is the persistent-config editor. It is opened from the picker
// when the user presses `s`. Callers read Persisted() after the program
// exits, which always reflects what's on disk — never unsaved edits.
type SettingsModel struct {
	initial    appconfig.File // last-persisted state; updated on successful save
	current    appconfig.File // editing buffer; discarded on esc/ctrl+c
	envToken   string
	cursor     settingsField
	width      int
	height     int
	editing    bool
	buffer     string
	confirming bool // asking the user to save/discard before exit
	saveMsg    string
	errorMsg   string
	testing    bool
	testResult *testResult
	savedPath  string
}

// Persisted returns the settings as they exist on disk — i.e., the last
// successfully-saved state. Unsaved edits in the current buffer are deliberately
// not returned, so callers can't accidentally apply them to the running config.
func (m *SettingsModel) Persisted() appconfig.File { return m.initial }

// NewSettingsModel creates the settings editor with the given on-disk config
// and the $IVIDEO_HLS_TOKEN value (if any), which is shown but never persisted
// unless the user explicitly types it.
func NewSettingsModel(loaded appconfig.File, envToken, savedPath string) *SettingsModel {
	return &SettingsModel{
		initial:   loaded,
		current:   loaded,
		envToken:  envToken,
		savedPath: savedPath,
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

// updateConfirming handles the save/discard prompt that appears when the user
// tries to exit with unsaved changes. Three choices: s save & exit, d discard
// & exit, esc return to editing.
func (m *SettingsModel) updateConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s":
		m.save()
		if m.errorMsg != "" {
			// save failed — stay in the confirm dialog so the user sees why
			return m, nil
		}
		return m, tea.Quit
	case "d":
		m.current = m.initial // drop unsaved edits before the caller reads Persisted()
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
		m.buffer = m.current.DefaultSourceDir
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
		m.current.AuthMethod = appconfig.InferAuthMethod(m.current.RemoteURL, m.current.AuthMethod)
	case fieldToken:
		m.current.Token = strings.TrimSpace(m.buffer)
	case fieldPublicURL:
		m.current.PublicURLPattern = strings.TrimSpace(m.buffer)
	case fieldSourceDir:
		m.current.DefaultSourceDir = strings.TrimSpace(m.buffer)
	}
	m.editing = false
	m.buffer = ""
}

func (m *SettingsModel) toggleAuth() {
	if m.current.AuthMethod == appconfig.AuthHTTPS {
		m.current.AuthMethod = appconfig.AuthSSH
		return
	}
	m.current.AuthMethod = appconfig.AuthHTTPS
}

func (m *SettingsModel) toggleBoolField() {
	switch m.cursor {
	case fieldPreCompress:
		m.current.DefaultPreCompress = !m.current.DefaultPreCompress
	case fieldKeepSource:
		m.current.DefaultKeepSource = !m.current.DefaultKeepSource
	case fieldPush:
		m.current.DefaultPushDisabled = !m.current.DefaultPushDisabled
	case fieldCleanup:
		m.current.DefaultCleanupDisabled = !m.current.DefaultCleanupDisabled
	case fieldRecursive:
		m.current.DefaultRecursive = !m.current.DefaultRecursive
	case fieldReuseCompressed:
		m.current.ResumeReuseCompressed = !m.current.ResumeReuseCompressed
	case fieldAuthMethod:
		m.toggleAuth()
	}
}

func (m *SettingsModel) adjustField(delta int) {
	switch m.cursor {
	case fieldParallel:
		m.current.DefaultParallel = clampInt(m.current.DefaultParallel+delta, 0, 32)
	case fieldQuality:
		m.current.DefaultQuality = stepChoice(m.current.DefaultQuality, qualityChoices(), delta)
	case fieldCompression:
		m.current.DefaultCompression = stepChoice(m.current.DefaultCompression, compressionChoices(), delta)
	case fieldAuthMethod:
		// two-choice toggle; direction doesn't matter but a no-op when the
		// target already matches prevents the double-flip on a held key.
		target := appconfig.AuthSSH
		if delta > 0 {
			target = appconfig.AuthHTTPS
		}
		m.current.AuthMethod = target
	}
}

func (m *SettingsModel) save() {
	if err := appconfig.ValidateRemoteURL(m.current.RemoteURL); err != nil {
		m.errorMsg = err.Error()
		m.saveMsg = ""
		return
	}
	if err := appconfig.Save(m.current); err != nil {
		m.errorMsg = err.Error()
		m.saveMsg = ""
		return
	}
	m.initial = m.current
	m.saveMsg = "saved → " + m.savedPath
	m.errorMsg = ""
}

func (m *SettingsModel) resetDefaults() {
	m.current = appconfig.File{
		RemoteURL:          "git@github.com:username/repo.git",
		AuthMethod:         appconfig.AuthSSH,
		DefaultQuality:     "medium",
		DefaultCompression: "balanced",
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
	url := appconfig.EffectiveRemoteURL(m.current.RemoteURL, token, m.current.AuthMethod)
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

// testRemote runs `git ls-remote` against the URL with a 10s timeout. The
// URL is expected to already have credentials baked in when needed.
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

// ---------- rendering ----------

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
		m.renderRow(fieldRecursive, "Recursive scan", boolLabel(m.current.DefaultRecursive)),
		"",
		m.renderGroupTitle("Defaults"),
		m.renderRow(fieldParallel, "Parallel jobs", m.renderParallelValue()),
		m.renderRow(fieldQuality, "Quality", choiceValue(m.current.DefaultQuality, "medium")),
		m.renderRow(fieldCompression, "Compression", choiceValue(m.current.DefaultCompression, "balanced")),
		m.renderRow(fieldPreCompress, "Pre-compress", boolLabel(m.current.DefaultPreCompress)),
		m.renderRow(fieldKeepSource, "Keep source .mp4", boolLabel(m.current.DefaultKeepSource)),
		m.renderRow(fieldPush, "Push to remote", boolLabel(!m.current.DefaultPushDisabled)),
		m.renderRow(fieldCleanup, "Clean up workspace", boolLabel(!m.current.DefaultCleanupDisabled)),
		m.renderRow(fieldReuseCompressed, "Reuse compressed on resume", boolLabel(m.current.ResumeReuseCompressed)),
	}

	hint := styleMuted.Render("ℹ " + fieldHint(m.cursor))
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
	if m.current.AuthMethod == appconfig.AuthHTTPS {
		return styleDim.Render(ssh) + "  /  " + styleAccent.Render(https)
	}
	return styleAccent.Render(ssh) + "  /  " + styleDim.Render(https)
}

func (m *SettingsModel) renderParallelValue() string {
	n := m.current.DefaultParallel
	if n <= 1 {
		return styleDim.Render("serial (1)") + "  " + styleMuted.Render(fmt.Sprintf("· %d CPU cores available", runtime.NumCPU()))
	}
	return fmt.Sprintf("%d concurrent", n) + "  " + styleMuted.Render(fmt.Sprintf("· %d CPU cores available · push pool %d", runtime.NumCPU(), n*2))
}

func (m *SettingsModel) renderSourceDirValue() string {
	if m.editing && m.cursor == fieldSourceDir {
		return m.buffer + styleAccent.Render("▌")
	}
	if m.current.DefaultSourceDir == "" {
		return styleDim.Render("(current working directory)")
	}
	return m.current.DefaultSourceDir
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
	return appconfig.MaskToken(token) + " " + styleDim.Render("("+sourceLabel(source)+")")
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

func fieldHint(f settingsField) string {
	switch f {
	case fieldURL:
		return "Where `git push` sends commits. SSH (git@host:path) or HTTPS (https://host/path)."
	case fieldAuthMethod:
		return "SSH uses your key agent. HTTPS injects the token into the push URL."
	case fieldToken:
		return "GitHub PAT or equivalent. Stored 0600; $IVIDEO_HLS_TOKEN overrides."
	case fieldPublicURL:
		return "HTTP(S) template for urls.txt — how viewers fetch the playlist. {branch}, {subdir}, {filename}. Empty = write local path."
	case fieldSourceDir:
		return "Folder scanned when no --source / -i / -a is given. Empty = cwd. Auto-created if set."
	case fieldRecursive:
		return "Walk subdirectories. Skips .git, node_modules, hero*, hidden dirs."
	case fieldParallel:
		return "Concurrent encodes. Push pool is auto-sized to 2× this (network is cheap)."
	case fieldReuseCompressed:
		return "When ON, resume-failed skips compress if a clean _compressed.mp4 is present (no .partial sibling)."
	case fieldPush:
		return "When OFF, commit locally but skip `git push`. Useful for dry-run or manual review."
	case fieldCleanup:
		return "When OFF, keep hero_<name>/ workspace after success so you can inspect it."
	case fieldQuality:
		return "Default output resolution/bitrate when the flag isn't passed."
	case fieldCompression:
		return "Default encoder preset. best = smallest but slowest."
	case fieldPreCompress:
		return "When ON, runs a compression pass before HLS by default."
	case fieldKeepSource:
		return "When ON, original .mp4 is kept after success by default."
	}
	return ""
}

func qualityChoices() []string    { return []string{"low", "medium", "high"} }
func compressionChoices() []string { return []string{"fast", "balanced", "best"} }

func choiceValue(current, fallback string) string {
	if current == "" {
		return styleDim.Render(fallback + " (default)")
	}
	return current
}

// RunSettings opens the settings editor as an alt-screen TUI and returns
// the last-persisted config after the user exits. Unsaved edits in the buffer
// are deliberately dropped so callers cannot confuse "typed" with "saved".
func RunSettings(loaded appconfig.File, envToken, savedPath string) (appconfig.File, error) {
	m := NewSettingsModel(loaded, envToken, savedPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return loaded, err
	}
	return m.Persisted(), nil
}

func stepChoice(current string, choices []string, delta int) string {
	idx := 0
	for i, c := range choices {
		if c == current {
			idx = i
			break
		}
	}
	idx = clampInt(idx+delta, 0, len(choices)-1)
	return choices[idx]
}
