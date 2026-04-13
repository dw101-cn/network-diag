# Sysmon TUI - System Monitor Design

## Overview

Replace the existing network-diag tool with a real-time system monitor TUI application. The tool displays CPU usage, memory usage, and network throughput, refreshing every second. Built with bubbletea (Charm ecosystem) and gopsutil, targeting Windows only.

## Project Structure

```
network-diag/
├── cmd/
│   └── sysmon/
│       └── main.go           # Entry point, initialize bubbletea
├── internal/
│   ├── collector/
│   │   └── collector.go      # Data collection (CPU/Memory/Network)
│   └── ui/
│       └── ui.go             # TUI model (bubbletea Model/Update/View)
├── go.mod
├── go.sum
└── .github/
    └── workflows/
        └── release.yml       # Updated build target
```

- Delete `cmd/network-diag/main.go`
- `internal/collector` wraps gopsutil calls, exposes a unified `Stats` struct
- `internal/ui` implements bubbletea's Model/Update/View
- `cmd/sysmon/main.go` initializes and starts the program

## Data Collection Layer (collector)

### Stats Struct

```go
type Stats struct {
    CPUPercent   float64   // Overall CPU usage 0-100
    MemTotal     uint64    // Total memory (bytes)
    MemUsed      uint64    // Used memory (bytes)
    MemPercent   float64   // Memory usage 0-100
    NetSentRate  uint64    // Upload rate (bytes/sec)
    NetRecvRate  uint64    // Download rate (bytes/sec)
}
```

### Data Sources

- **CPU**: `cpu.Percent(0, false)` — overall percentage, gopsutil handles internal sampling
- **Memory**: `mem.VirtualMemory()` — reads Total / Used / UsedPercent
- **Network**: `net.IOCounters(false)` — cumulative bytes; collector stores previous sample and computes rate as delta / elapsed time

### Public API

```go
func Collect() (Stats, error)
```

Called once per second by the UI tick timer.

## TUI Layer (ui)

### Model

```go
type Model struct {
    stats     collector.Stats  // Latest sample
    err       error            // Collection error
    width     int              // Terminal width (responds to resize)
    quitting  bool
}
```

### Messages and Commands

- `tickMsg` — fires every second, calls `collector.Collect()` for new data
- `tea.WindowSizeMsg` — updates width on terminal resize, progress bars adapt

### Update Logic

- `tickMsg` received → update `stats`, schedule next tick
- `q` / `ctrl+c` / `esc` received → quit

### View Rendering

Uses lipgloss for styling. Target layout:

```
╭─ System Monitor ────────────────────╮
│                                     │
│  CPU   45.2%  ████████░░░░░░░░░░░░  │
│  MEM   8.1 / 16.0 GB  (50.6%)      │
│        █████████░░░░░░░░░░░░░       │
│  NET   ↑ 1.2 MB/s  ↓ 5.8 MB/s      │
│                                     │
│  Press q to quit                    │
╰─────────────────────────────────────╯
```

- Progress bars built with `█` and `░`, width adapts to terminal size
- Network rate auto-selects unit (B/s, KB/s, MB/s, GB/s)
- Title and border use lipgloss `Border` style

## Entry Point and Build

### cmd/sysmon/main.go

```go
func main() {
    m := ui.NewModel()
    p := tea.NewProgram(m, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

- `tea.WithAltScreen()` — uses alternate screen buffer, terminal restores cleanly on exit
- Double-clicking exe opens a cmd window where bubbletea renders normally

### CI Build Updates (release.yml)

- Build path: `./cmd/network-diag/` → `./cmd/sysmon/`
- Artifact name: `network-diag-*` → `sysmon-*`
- Targets unchanged: Windows amd64 + arm64 only

### New go.mod Dependencies

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/lipgloss`
- `github.com/shirou/gopsutil/v4`

## Out of Scope

- No existing network diagnostic features retained (full replacement)
- No cross-platform support (Windows only)
- No detailed panel (simple dashboard first, extend later)
- No log/file output (pure terminal display)
