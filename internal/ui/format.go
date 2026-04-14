package ui

import (
	"fmt"
	"strings"

	"github.com/dw101-cn/network-diag/internal/collector"
)

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

func progressBar(percent float64, width int) string {
	filled := int(percent/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func formatProcessLine(proc collector.ProcessInfo, barWidth int) string {
	name := proc.Name
	if len(name) > 15 {
		name = name[:12] + "..."
	}
	cpuBar := progressBar(proc.CPUPercent, barWidth/2)
	return fmt.Sprintf("  %-15s  %5.1f%%  %s  %s\n",
		name,
		proc.CPUPercent,
		cpuBar,
		formatBytes(proc.MemUsedBytes),
	)
}
