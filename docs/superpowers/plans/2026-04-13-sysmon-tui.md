# Sysmon TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the network-diag tool with a real-time system monitor TUI that displays CPU, memory, and network throughput, refreshing every second.

**Architecture:** Three-layer design — `collector` gathers system stats via gopsutil, `ui` renders them via bubbletea/lipgloss, `main` wires them together. The collector is a stateful struct that tracks previous network counters to compute rates.

**Tech Stack:** Go 1.24, bubbletea, lipgloss, gopsutil/v4

---

### File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `cmd/sysmon/main.go` | Entry point, start bubbletea |
| Create | `internal/collector/collector.go` | System stats collection |
| Create | `internal/collector/collector_test.go` | Collector tests |
| Create | `internal/ui/ui.go` | Bubbletea Model/Update/View |
| Create | `internal/ui/format.go` | Progress bar and unit formatting helpers |
| Create | `internal/ui/format_test.go` | Format helper tests |
| Modify | `go.mod` | Add dependencies |
| Modify | `.github/workflows/release.yml` | Update build path and artifact name |
| Delete | `cmd/network-diag/main.go` | Remove old tool |

---

### Task 1: Project Scaffolding

**Files:**
- Modify: `go.mod`
- Create directories: `cmd/sysmon/`, `internal/collector/`, `internal/ui/`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p cmd/sysmon internal/collector internal/ui
```

- [ ] **Step 2: Update go.mod and add dependencies**

Update `go.mod` module name stays the same. Add dependencies:

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/shirou/gopsutil/v4@latest
```

- [ ] **Step 3: Create a minimal main.go to verify build**

Create `cmd/sysmon/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("sysmon placeholder")
}
```

- [ ] **Step 4: Verify build works**

Run: `go build ./cmd/sysmon/`
Expected: builds without errors

- [ ] **Step 5: Commit**

```bash
git add cmd/sysmon/main.go go.mod go.sum
git commit -m "chore: scaffold sysmon project with dependencies"
```

---

### Task 2: Format Helpers (TDD)

**Files:**
- Create: `internal/ui/format.go`
- Create: `internal/ui/format_test.go`

These are pure functions with no dependencies — ideal to build and test first.

- [ ] **Step 1: Write failing tests for formatRate**

Create `internal/ui/format_test.go`:

```go
package ui

import "testing"

func TestFormatRate(t *testing.T) {
	tests := []struct {
		bytesPerSec uint64
		want        string
	}{
		{0, "0 B/s"},
		{512, "512 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"},
		{1048576, "1.0 MB/s"},
		{1572864, "1.5 MB/s"},
		{1073741824, "1.0 GB/s"},
	}
	for _, tt := range tests {
		got := formatRate(tt.bytesPerSec)
		if got != tt.want {
			t.Errorf("formatRate(%d) = %q, want %q", tt.bytesPerSec, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestFormatRate -v`
Expected: FAIL — `formatRate` not defined

- [ ] **Step 3: Implement formatRate**

Create `internal/ui/format.go`:

```go
package ui

import "fmt"

func formatRate(bytesPerSec uint64) string {
	switch {
	case bytesPerSec >= 1<<30:
		return fmt.Sprintf("%.1f GB/s", float64(bytesPerSec)/float64(1<<30))
	case bytesPerSec >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", float64(bytesPerSec)/float64(1<<20))
	case bytesPerSec >= 1<<10:
		return fmt.Sprintf("%.1f KB/s", float64(bytesPerSec)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestFormatRate -v`
Expected: PASS

- [ ] **Step 5: Write failing tests for formatBytes**

Add to `internal/ui/format_test.go`:

```go
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1048576, "1.0 MB"},
		{8589934592, "8.0 GB"},      // 8 GB
		{17179869184, "16.0 GB"},    // 16 GB
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestFormatBytes -v`
Expected: FAIL — `formatBytes` not defined

- [ ] **Step 7: Implement formatBytes**

Add to `internal/ui/format.go`:

```go
func formatBytes(bytes uint64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestFormatBytes -v`
Expected: PASS

- [ ] **Step 9: Write failing tests for progressBar**

Add to `internal/ui/format_test.go`:

```go
func TestProgressBar(t *testing.T) {
	tests := []struct {
		percent float64
		width   int
		want    string
	}{
		{0, 10, "░░░░░░░░░░"},
		{100, 10, "██████████"},
		{50, 10, "█████░░░░░"},
		{33.3, 9, "███░░░░░░"},
	}
	for _, tt := range tests {
		got := progressBar(tt.percent, tt.width)
		if got != tt.want {
			t.Errorf("progressBar(%.1f, %d) = %q, want %q", tt.percent, tt.width, got, tt.want)
		}
	}
}
```

- [ ] **Step 10: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestProgressBar -v`
Expected: FAIL — `progressBar` not defined

- [ ] **Step 11: Implement progressBar**

Add to `internal/ui/format.go`:

```go
import "strings"

func progressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
```

Note: update the import block to include `"strings"` alongside `"fmt"`.

- [ ] **Step 12: Run all format tests**

Run: `go test ./internal/ui/ -v`
Expected: all PASS

- [ ] **Step 13: Commit**

```bash
git add internal/ui/format.go internal/ui/format_test.go
git commit -m "feat: add format helpers for progress bar and unit display"
```

---

### Task 3: Collector

**Files:**
- Create: `internal/collector/collector.go`
- Create: `internal/collector/collector_test.go`

- [ ] **Step 1: Write failing test for Collector**

Create `internal/collector/collector_test.go`:

```go
package collector

import "testing"

func TestCollect(t *testing.T) {
	c := New()

	stats, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	// CPU percent should be in range 0-100
	if stats.CPUPercent < 0 || stats.CPUPercent > 100 {
		t.Errorf("CPUPercent = %.2f, want 0-100", stats.CPUPercent)
	}

	// Memory total should be > 0
	if stats.MemTotal == 0 {
		t.Error("MemTotal = 0, want > 0")
	}

	// MemUsed should be <= MemTotal
	if stats.MemUsed > stats.MemTotal {
		t.Errorf("MemUsed (%d) > MemTotal (%d)", stats.MemUsed, stats.MemTotal)
	}

	// MemPercent should be in range 0-100
	if stats.MemPercent < 0 || stats.MemPercent > 100 {
		t.Errorf("MemPercent = %.2f, want 0-100", stats.MemPercent)
	}

	// First call: network rates should be 0 (no previous sample)
	if stats.NetSentRate != 0 || stats.NetRecvRate != 0 {
		t.Errorf("First call rates = (%d, %d), want (0, 0)", stats.NetSentRate, stats.NetRecvRate)
	}
}

func TestCollectNetworkRate(t *testing.T) {
	c := New()

	// First call establishes baseline
	_, err := c.Collect()
	if err != nil {
		t.Fatalf("First Collect() error: %v", err)
	}

	// Second call should have rates >= 0 (can't guarantee > 0 on CI)
	stats, err := c.Collect()
	if err != nil {
		t.Fatalf("Second Collect() error: %v", err)
	}

	// Just verify it doesn't panic or return nonsensical values
	// Rates are uint64, so they can't be negative
	_ = stats.NetSentRate
	_ = stats.NetRecvRate
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/ -v`
Expected: FAIL — package not found

- [ ] **Step 3: Implement Collector**

Create `internal/collector/collector.go`:

```go
package collector

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Stats holds a single sample of system metrics.
type Stats struct {
	CPUPercent  float64 // Overall CPU usage 0-100
	MemTotal    uint64  // Total memory (bytes)
	MemUsed     uint64  // Used memory (bytes)
	MemPercent  float64 // Memory usage 0-100
	NetSentRate uint64  // Upload rate (bytes/sec)
	NetRecvRate uint64  // Download rate (bytes/sec)
}

// Collector gathers system stats, tracking previous network counters for rate calculation.
type Collector struct {
	prevNetSent uint64
	prevNetRecv uint64
	prevTime    time.Time
	initialized bool
}

// New creates a new Collector.
func New() *Collector {
	return &Collector{}
}

// Collect gathers current system stats. The first call returns zero network rates
// because there is no previous sample to compute a delta from.
func (c *Collector) Collect() (Stats, error) {
	var s Stats

	// CPU
	percents, err := cpu.Percent(0, false)
	if err != nil {
		return s, err
	}
	if len(percents) > 0 {
		s.CPUPercent = percents[0]
	}

	// Memory
	vmem, err := mem.VirtualMemory()
	if err != nil {
		return s, err
	}
	s.MemTotal = vmem.Total
	s.MemUsed = vmem.Used
	s.MemPercent = vmem.UsedPercent

	// Network
	counters, err := net.IOCounters(false)
	if err != nil {
		return s, err
	}
	if len(counters) > 0 {
		now := time.Now()
		sent := counters[0].BytesSent
		recv := counters[0].BytesRecv

		if c.initialized {
			elapsed := now.Sub(c.prevTime).Seconds()
			if elapsed > 0 {
				s.NetSentRate = uint64(float64(sent-c.prevNetSent) / elapsed)
				s.NetRecvRate = uint64(float64(recv-c.prevNetRecv) / elapsed)
			}
		}

		c.prevNetSent = sent
		c.prevNetRecv = recv
		c.prevTime = now
		c.initialized = true
	}

	return s, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/collector.go internal/collector/collector_test.go
git commit -m "feat: add system stats collector using gopsutil"
```

---

### Task 4: TUI Model

**Files:**
- Create: `internal/ui/ui.go`

- [ ] **Step 1: Implement the bubbletea Model**

Create `internal/ui/ui.go`:

```go
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

	content := fmt.Sprintf("%s\n\n%s\n%s\n%s\n%s\n\n%s",
		title, cpuLine, memLine, memBarLine, netLine, hint)

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
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/ui/`
Expected: builds without errors

- [ ] **Step 3: Commit**

```bash
git add internal/ui/ui.go
git commit -m "feat: add bubbletea TUI model with system monitor view"
```

---

### Task 5: Entry Point

**Files:**
- Modify: `cmd/sysmon/main.go`

- [ ] **Step 1: Replace placeholder main.go**

Replace `cmd/sysmon/main.go` with:

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dw101-cn/network-diag/internal/ui"
)

func main() {
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build and run locally**

Run: `go build -o sysmon ./cmd/sysmon/ && ./sysmon`
Expected: TUI appears showing CPU, memory, network stats. Press `q` to exit.

- [ ] **Step 3: Run all tests**

Run: `go test ./... -v`
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/sysmon/main.go
git commit -m "feat: add sysmon entry point wiring collector and TUI"
```

---

### Task 6: CI and Cleanup

**Files:**
- Modify: `.github/workflows/release.yml`
- Delete: `cmd/network-diag/main.go`

- [ ] **Step 1: Delete old network-diag code**

```bash
rm cmd/network-diag/main.go
rmdir cmd/network-diag
```

- [ ] **Step 2: Update release.yml**

Replace `.github/workflows/release.yml` with:

```yaml
name: Build and Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: windows
            goarch: amd64
            suffix: .exe
          - goos: windows
            goarch: arm64
            suffix: .exe
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Build sysmon
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          go build -ldflags="-s -w" -o sysmon-${{ matrix.goos }}-${{ matrix.goarch }}${{ matrix.suffix }} ./cmd/sysmon/

      - uses: actions/upload-artifact@v4
        with:
          name: sysmon-${{ matrix.goos }}-${{ matrix.goarch }}
          path: sysmon-*

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          merge-multiple: true

      - name: Create Release
        uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
          files: |
            sysmon-*
```

- [ ] **Step 3: Verify build still works**

Run: `go build ./cmd/sysmon/`
Expected: builds without errors

- [ ] **Step 4: Run all tests**

Run: `go test ./... -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: remove network-diag, update CI for sysmon"
```
