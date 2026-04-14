package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dw101-cn/network-diag/internal/collector"
)

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Width(5).
			Align(lipgloss.Right)

	barFillStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	barEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray

	uploadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("213")) // pink
	downloadStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // blue

	hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Model is the bubbletea model for the system monitor.
type Model struct {
	stats     collector.Stats
	collector *collector.Collector
	err       error
	width     int
	quitting  bool
}

type tickMsg struct {
	stats collector.Stats
	err   error
}

// NewModel creates a new Model with an initialized collector.
func NewModel() Model {
	return Model{
		collector: collector.New(),
		width:     60,
	}
}

func (m Model) doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		stats, err := m.collector.Collect()
		return tickMsg{stats: stats, err: err}
	})
}

// Init starts the first tick.
func (m Model) Init() tea.Cmd {
	return m.doTick()
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tickMsg:
		m.stats = msg.stats
		m.err = msg.err
		return m, m.doTick()
	}

	return m, nil
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	barWidth := m.width - 30
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 40 {
		barWidth = 40
	}

	s := m.stats

	title := titleStyle.Render("System Monitor")

	cpuFilled := filled(s.CPUPercent, barWidth)
	cpuBar := barFillStyle.Render(strings.Repeat("█", cpuFilled)) +
		barEmptyStyle.Render(strings.Repeat("░", barWidth-cpuFilled))

	cpuLine := fmt.Sprintf("%s  %5.1f%%  %s",
		labelStyle.Render("CPU"),
		s.CPUPercent,
		cpuBar,
	)

	var processLines string
	if len(s.TopProcesses) > 0 {
		processLines = "\n" + titleStyle.Render("Top Processes") + "\n"
		for _, proc := range s.TopProcesses {
			processLines += formatProcessLine(proc, barWidth)
		}
	}

	memLine := fmt.Sprintf("%s  %s / %s  (%.1f%%)",
		labelStyle.Render("MEM"),
		formatBytes(s.MemUsed),
		formatBytes(s.MemTotal),
		s.MemPercent,
	)

	memFilled := filled(s.MemPercent, barWidth)
	memBar := barFillStyle.Render(strings.Repeat("█", memFilled)) +
		barEmptyStyle.Render(strings.Repeat("░", barWidth-memFilled))

	memBarLine := fmt.Sprintf("%s  %s", labelStyle.Render(""), memBar)

	netLine := fmt.Sprintf("%s  %s %s  %s %s",
		labelStyle.Render("NET"),
		uploadStyle.Render("↑"),
		formatRate(s.NetSentRate),
		downloadStyle.Render("↓"),
		formatRate(s.NetRecvRate),
	)

	hint := hintStyle.Render("Press q to quit")

	content := fmt.Sprintf("%s\n\n%s\n%s\n%s\n%s\n%s\n\n%s",
		title, cpuLine, memLine, memBarLine, netLine, processLines, hint)

	if m.err != nil {
		errMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(
			fmt.Sprintf("Error: %v", m.err))
		content += "\n" + errMsg
	}

	return borderStyle.Render(content) + "\n"
}

func filled(percent float64, width int) int {
	n := int(percent / 100 * float64(width))
	if n > width {
		n = width
	}
	if n < 0 {
		n = 0
	}
	return n
}
