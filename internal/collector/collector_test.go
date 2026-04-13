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
