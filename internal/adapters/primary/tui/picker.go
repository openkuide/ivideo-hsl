package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chamrong/ivideo-hls/internal/app"
	"github.com/chamrong/ivideo-hls/internal/domain/settings"
	"github.com/chamrong/ivideo-hls/internal/domain/video"
)

type screen int

const (
	screenPick screen = iota
	screenConfig
)

const (
	cfgFieldParallel = iota
	cfgFieldQuality
	cfgFieldCompression
	cfgFieldPreCompress
	cfgFieldKeepSource
	cfgFieldCount
)

type videoItem struct {
	name    string
	path    string
	size    int64
	checked bool
}

// PickerModel is the interactive video-selection and run-config screen.
// After the program exits, callers read Confirmed and SelectedVideos.
type PickerModel struct {
	screen     screen
	items      []videoItem
	cursor     int
	workingDir string
	recursive  bool
	width      int
	height     int

	// config state
	parallel     int
	quality      int // 0=low,1=medium,2=high
	compression  int // 0=fast,1=balanced,2=best
	preCompress  bool
	keepSource   bool
	configCursor int

	// filter state
	filtering   bool
	filterInput string

	cfg  settings.Settings
	a    *app.App
	done bool // set by commitConfig

	// Confirmed is true when the user pressed enter on the config screen.
	Confirmed bool
	// SelectedVideos is populated on confirmation.
	SelectedVideos []string
	// UpdatedSettings holds the run-config choices the user made.
	UpdatedSettings settings.Settings

	// Banner is an optional one-line notice rendered above the video list.
	Banner string

	// WantSettings is set when the user presses `s` on the picker.
	WantSettings bool
}

// NewPicker creates a picker model loaded with the given *app.App and initial settings.
func NewPicker(a *app.App, cfg settings.Settings) (*PickerModel, error) {
	sourceDir := cfg.SourceDir
	if sourceDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		sourceDir = wd
	}
	items, err := discoverVideos(a, sourceDir, cfg.Recursive)
	if err != nil {
		return nil, err
	}
	return &PickerModel{
		items:       items,
		workingDir:  sourceDir,
		recursive:   cfg.Recursive,
		parallel:    initialParallel(cfg),
		quality:     qualityIndex(cfg.Quality),
		compression: compressionIndex(cfg.Compression),
		preCompress: cfg.PreCompress,
		keepSource:  cfg.KeepSource,
		cfg:         cfg,
		a:           a,
	}, nil
}

func initialParallel(cfg settings.Settings) int {
	if cfg.MaxParallel >= 1 {
		return cfg.MaxParallel
	}
	return 1
}

func qualityIndex(q video.Quality) int {
	switch q {
	case video.QualityLow:
		return 0
	case video.QualityHigh:
		return 2
	}
	return 1 // medium is the default
}

func compressionIndex(c video.Compression) int {
	switch c {
	case video.CompressionFast:
		return 0
	case video.CompressionBest:
		return 2
	}
	return 1 // balanced is the default
}

func discoverVideos(a *app.App, dir string, recursive bool) ([]videoItem, error) {
	paths, err := a.Scanner.Scan(dir, recursive)
	if err != nil {
		return nil, err
	}
	items := make([]videoItem, 0, len(paths))
	for _, v := range paths {
		info, err := os.Stat(v.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dir, v.Path)
		if err != nil {
			rel = filepath.Base(v.Path)
		}
		items = append(items, videoItem{
			name: rel,
			path: v.Path,
			size: info.Size(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return naturalLess(items[i].name, items[j].name)
	})
	return items, nil
}

func (m *PickerModel) Init() tea.Cmd { return nil }

func (m *PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch m.screen {
		case screenPick:
			return m.updatePick(msg)
		case screenConfig:
			return m.updateConfig(msg)
		}
	}
	return m, nil
}

func (m *PickerModel) visibleItems() []int {
	if !m.filtering && m.filterInput == "" {
		out := make([]int, len(m.items))
		for i := range m.items {
			out[i] = i
		}
		return out
	}
	needle := strings.ToLower(m.filterInput)
	var out []int
	for i, it := range m.items {
		if strings.Contains(strings.ToLower(it.name), needle) {
			out = append(out, i)
		}
	}
	return out
}

func (m *PickerModel) cursorItemIdx() int {
	vis := m.visibleItems()
	if len(vis) == 0 {
		return -1
	}
	if m.cursor >= len(vis) {
		m.cursor = len(vis) - 1
	}
	return vis[m.cursor]
}

func (m *PickerModel) updatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.updateFilter(msg)
	}
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "/":
		m.filtering = true
		return m, nil
	case "r":
		items, err := discoverVideos(m.a, m.workingDir, m.recursive)
		if err != nil {
			m.Banner = "rescan failed: " + err.Error()
		} else {
			m.items = items
			m.cursor = 0
			m.Banner = ""
		}
		return m, nil
	case "s":
		m.WantSettings = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visibleItems())-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(len(m.visibleItems())-1, 0)
	case " ":
		if idx := m.cursorItemIdx(); idx >= 0 {
			m.items[idx].checked = !m.items[idx].checked
		}
	case "a":
		m.toggleAllVisible()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.selectFirstN(int(msg.Runes[0] - '0'))
	case "enter":
		if len(m.selectedPaths()) == 0 {
			return m, nil
		}
		m.screen = screenConfig
	}
	return m, nil
}

func (m *PickerModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterInput = ""
		m.cursor = 0
	case "enter":
		m.filtering = false
		m.cursor = 0
	case "backspace":
		if n := len(m.filterInput); n > 0 {
			m.filterInput = m.filterInput[:n-1]
		}
		m.cursor = 0
	default:
		if len(msg.Runes) > 0 {
			m.filterInput += string(msg.Runes)
			m.cursor = 0
		}
	}
	return m, nil
}

func (m *PickerModel) toggleAllVisible() {
	vis := m.visibleItems()
	allChecked := true
	for _, i := range vis {
		if !m.items[i].checked {
			allChecked = false
			break
		}
	}
	for _, i := range vis {
		m.items[i].checked = !allChecked
	}
}

func (m *PickerModel) selectFirstN(n int) {
	vis := m.visibleItems()
	n = min(n, len(vis))
	for pos, idx := range vis {
		m.items[idx].checked = pos < n
	}
}

func (m *PickerModel) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenPick
	case "up", "k":
		if m.configCursor > 0 {
			m.configCursor--
		}
	case "down", "j":
		if m.configCursor < cfgFieldCount-1 {
			m.configCursor++
		}
	case "left", "h":
		m.configAdjust(-1)
	case "right", "l":
		m.configAdjust(1)
	case " ":
		m.toggleBooleanField()
	case "enter":
		m.commitConfig()
		return m, tea.Quit
	}
	return m, nil
}

func (m *PickerModel) toggleBooleanField() {
	switch m.configCursor {
	case cfgFieldPreCompress:
		m.preCompress = !m.preCompress
	case cfgFieldKeepSource:
		m.keepSource = !m.keepSource
	}
}

func (m *PickerModel) commitConfig() {
	qualities := []video.Quality{video.QualityLow, video.QualityMedium, video.QualityHigh}
	compressions := []video.Compression{video.CompressionFast, video.CompressionBalanced, video.CompressionBest}

	m.SelectedVideos = m.selectedPaths()
	m.UpdatedSettings = m.cfg
	m.UpdatedSettings.MaxParallel = m.parallel
	m.UpdatedSettings.ParallelMode = m.parallel > 1
	m.UpdatedSettings.Quality = qualities[m.quality]
	m.UpdatedSettings.Compression = compressions[m.compression]
	m.UpdatedSettings.PreCompress = m.preCompress
	m.UpdatedSettings.KeepSource = m.keepSource
	m.Confirmed = true

	// Persist run-config choices; failures are non-fatal.
	if m.a != nil {
		_ = m.a.Config.SaveRunConfig(
			m.UpdatedSettings.Quality,
			m.UpdatedSettings.Compression,
			m.UpdatedSettings.MaxParallel,
			m.UpdatedSettings.PreCompress,
			m.UpdatedSettings.KeepSource,
		)
	}
}

func (m *PickerModel) configAdjust(delta int) {
	switch m.configCursor {
	case cfgFieldParallel:
		limit := max(len(m.selectedPaths()), 1)
		m.parallel = clampInt(m.parallel+delta, 1, limit)
	case cfgFieldQuality:
		m.quality = clampInt(m.quality+delta, 0, 2)
	case cfgFieldCompression:
		m.compression = clampInt(m.compression+delta, 0, 2)
	case cfgFieldPreCompress:
		m.preCompress = !m.preCompress
	case cfgFieldKeepSource:
		m.keepSource = !m.keepSource
	}
}

func (m *PickerModel) selectedPaths() []string {
	var out []string
	for _, it := range m.items {
		if it.checked {
			out = append(out, it.path)
		}
	}
	return out
}

func (m *PickerModel) View() string {
	header := m.renderHeader()
	switch m.screen {
	case screenPick:
		return header + "\n" + m.viewPick()
	case screenConfig:
		return header + "\n" + m.viewConfig()
	}
	return ""
}

func (m *PickerModel) renderHeader() string {
	title := styleTitle.Render(" ivideo-hls ")
	sub := styleSubtitle.Render("  HLS video pipeline " + iconSpark)
	return title + sub
}

func (m *PickerModel) viewPick() string {
	if len(m.items) == 0 {
		return m.renderEmptyDirPanel()
	}
	vis := m.visibleItems()
	if len(vis) == 0 {
		return m.renderEmptyFilterPanel()
	}
	rows, selectedVisible := m.renderPickRows(vis)
	list := strings.Join(rows, "\n")
	countLine := fmt.Sprintf("%s %s",
		styleBadge.Render(fmt.Sprintf(" %d / %d ", m.totalSelected(), len(m.items))),
		styleMuted.Render(fmt.Sprintf("selected · %d shown", selectedVisible)))
	filterLine := m.renderFilterLine()
	help := styleHelp.Render(m.pickHelpText())
	body := m.renderSourceHeader() + "\n"
	if m.Banner != "" {
		body += styleWarn.Render(m.Banner) + "\n"
	}
	if filterLine != "" {
		body += filterLine + "\n"
	}
	body += "\n" + list + "\n\n" + countLine
	panel := stylePanel.Width(m.panelWidth()).Render(body)
	return panel + "\n" + help
}

func (m *PickerModel) renderSourceHeader() string {
	parts := []string{
		styleAccent.Render("📼 Videos in "),
		styleDim.Render(m.workingDir),
	}
	mode := "flat"
	if m.recursive {
		mode = "recursive"
	}
	parts = append(parts, styleDim.Render("  ·  "+mode))
	return strings.Join(parts, "")
}

func (m *PickerModel) renderEmptyDirPanel() string {
	body := m.renderSourceHeader() + "\n"
	if m.Banner != "" {
		body += styleWarn.Render(m.Banner) + "\n"
	}
	body += "\n" + styleError.Render("No video files found in this folder.") + "\n\n" +
		styleInfo.Render("What now?") + "\n" +
		styleMuted.Render("  • ") + styleKey("s") + styleMuted.Render(" — open settings (change default source folder)") + "\n" +
		styleMuted.Render("  • ") + styleKey("r") + styleMuted.Render(" — re-scan this folder") + "\n" +
		styleMuted.Render("  • ") + styleKey("q") + styleMuted.Render(" — quit") + "\n\n" +
		styleDim.Render("Or relaunch with --source /path/to/videos.")
	panel := stylePanel.Width(m.panelWidth()).Render(body)
	return panel + "\n" + styleHelp.Render("s settings · r rescan · q quit")
}

func styleKey(s string) string {
	return lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(s)
}

func (m *PickerModel) renderEmptyFilterPanel() string {
	body := m.renderSourceHeader() + "\n" +
		m.renderFilterLine() + "\n\n" +
		styleDim.Render("no matches") + "\n"
	panel := stylePanel.Width(m.panelWidth()).Render(body)
	help := styleHelp.Render("esc clear filter · q quit")
	return panel + "\n" + help
}

func (m *PickerModel) renderFilterLine() string {
	if !m.filtering && m.filterInput == "" {
		return ""
	}
	label := styleDim.Render("filter:")
	cursor := ""
	if m.filtering {
		cursor = styleAccent.Render("▌")
	}
	return label + " " + styleInfo.Render(m.filterInput) + cursor
}

func (m *PickerModel) renderPickRows(vis []int) (rows []string, selectedVisible int) {
	rows = make([]string, 0, len(vis))
	for visPos, itemIdx := range vis {
		it := m.items[itemIdx]
		rows = append(rows, m.renderPickRow(visPos, it))
		if it.checked {
			selectedVisible++
		}
	}
	return rows, selectedVisible
}

func (m *PickerModel) renderPickRow(visPos int, it videoItem) string {
	cursor := "  "
	if visPos == m.cursor && !m.filtering {
		cursor = styleAccent.Render(iconArrow + " ")
	}
	check := styleUnchecked.Render(iconCheckOff)
	if it.checked {
		check = styleCheck.Render(iconCheckOn)
	}
	name := it.name
	if visPos == m.cursor && !m.filtering {
		name = styleSelected.Render(" " + name + " ")
	} else {
		name = styleInfo.Render(name)
	}
	size := styleMuted.Render(fmt.Sprintf("%7s", humanBytes(it.size)))
	return fmt.Sprintf("%s%s %s  %s", cursor, check, size, name)
}

func (m *PickerModel) totalSelected() int {
	n := 0
	for _, it := range m.items {
		if it.checked {
			n++
		}
	}
	return n
}

func (m *PickerModel) pickHelpText() string {
	if m.filtering {
		return "type to filter · enter accept · esc cancel"
	}
	return "↑/↓ move · space toggle · a all · 1-9 first N · / filter · s settings · enter continue · q quit"
}

func (m *PickerModel) viewConfig() string {
	selected := m.selectedPaths()
	rows := []string{
		m.renderConfigRow(cfgFieldParallel, "Parallel jobs",
			fmt.Sprintf("%d / %d  (CPU cores: %d)", m.parallel, max(len(selected), 1), runtime.NumCPU())),
		m.renderConfigRow(cfgFieldQuality, "Quality", qualityLabel(m.quality)),
		m.renderConfigRow(cfgFieldCompression, "Compression", compressionLabel(m.compression)),
		m.renderConfigRow(cfgFieldPreCompress, "Pre-compress", boolLabel(m.preCompress)),
		m.renderConfigRow(cfgFieldKeepSource, "Keep source .mp4", boolLabel(m.keepSource)),
	}
	summary := styleDim.Render(fmt.Sprintf("%d video%s queued", len(selected), plural(len(selected))))
	hint := styleMuted.Render("ℹ " + configHint(m.configCursor))
	help := styleHelp.Render("↑/↓ field · ←/→ adjust · space toggle · enter start · esc back · q quit")
	panel := stylePanel.Width(m.panelWidth()).Render(
		styleAccent.Render("⚙  Configure pipeline") + "\n" +
			summary + "\n\n" +
			strings.Join(rows, "\n") + "\n\n" +
			hint,
	)
	return panel + "\n" + help
}

func (m *PickerModel) panelWidth() int {
	w := m.width - 2
	return clampInt(w, 60, 110)
}

func (m *PickerModel) renderConfigRow(field int, label, value string) string {
	active := m.configCursor == field
	labelStr := lipgloss.NewStyle().Width(18).Render(label)
	valueStyle := styleInfo
	marker := "  "
	if active {
		marker = styleAccent.Render(iconArrow + " ")
		valueStyle = styleAccent
	}
	return marker + styleMuted.Render(labelStr) + "  " + valueStyle.Render(value)
}

func configHint(field int) string {
	switch field {
	case cfgFieldParallel:
		return "Concurrent encodes (CPU pool)."
	case cfgFieldQuality:
		return "Output resolution & bitrate. Higher = larger files, sharper playback."
	case cfgFieldCompression:
		return "Encoder preset. 'best' is slowest but smallest; 'fast' is opposite."
	case cfgFieldPreCompress:
		return "Run an extra compression pass before HLS. Smaller segments, slower job."
	case cfgFieldKeepSource:
		return "When ON, the original .mp4 is kept after a successful run (default: deleted)."
	}
	return ""
}

func qualityLabel(i int) string {
	return []string{"low (480p · 800k)", "medium (720p · 2.8M)", "high (1080p · 5M)"}[i]
}

func compressionLabel(i int) string {
	return []string{"fast", "balanced", "best (slow)"}[i]
}

func boolLabel(b bool) string {
	if b {
		return styleCheck.Render(iconCheckOn + " on")
	}
	return styleUnchecked.Render(iconCheckOff + " off")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func humanBytes(b int64) string {
	const unit = 1024.0
	if b < int64(unit) {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := unit, 0
	for n := float64(b) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	const suffix = "KMGTPE"
	if exp >= len(suffix) {
		exp = len(suffix) - 1
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/div, suffix[exp])
}

func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ac, bc := a[i], b[j]
		if isDigit(ac) && isDigit(bc) {
			ai, al := readNum(a, i)
			bi, bl := readNum(b, j)
			if ai != bi {
				return ai < bi
			}
			i += al
			j += bl
			continue
		}
		if ac != bc {
			return lower(ac) < lower(bc)
		}
		i++
		j++
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

func readNum(s string, start int) (val int, length int) {
	for i := start; i < len(s); i++ {
		if !isDigit(s[i]) {
			break
		}
		val = val*10 + int(s[i]-'0')
		length++
	}
	return
}

// RunPicker runs the interactive video picker TUI and returns the picker model
// after completion. Callers read model.Confirmed, model.SelectedVideos, and
// model.WantSettings.
func RunPicker(a *app.App, cfg settings.Settings) (*PickerModel, error) {
	m, err := NewPicker(a, cfg)
	if err != nil {
		return nil, err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return nil, err
	}
	return m, nil
}
