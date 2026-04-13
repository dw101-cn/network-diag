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
