package ui

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1048576, "1.0 MB"},
		{8589934592, "8.0 GB"},   // 8 GB
		{17179869184, "16.0 GB"}, // 16 GB
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

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
