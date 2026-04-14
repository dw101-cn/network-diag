package collector

import (
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// ProcessInfo holds information about a single process.
type ProcessInfo struct {
	PID          int32   // Process ID
	Name         string  // Process name
	CPUPercent   float64 // CPU usage percentage
	MemPercent   float32 // Memory usage percentage
	MemUsedBytes uint64  // Memory used in bytes
}

// Stats holds a single sample of system metrics.
type Stats struct {
	CPUPercent   float64       // Overall CPU usage 0-100
	MemTotal     uint64        // Total memory (bytes)
	MemUsed      uint64        // Used memory (bytes)
	MemPercent   float64       // Memory usage 0-100
	NetSentRate  uint64        // Upload rate (bytes/sec)
	NetRecvRate  uint64        // Download rate (bytes/sec)
	TopProcesses []ProcessInfo // Top 5 processes by CPU usage
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

	s.TopProcesses = collectTopProcesses()

	return s, nil
}

// collectTopProcesses gathers top 5 processes by CPU usage.
func collectTopProcesses() []ProcessInfo {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}

	var procInfos []ProcessInfo
	for _, p := range procs {
		// Skip processes that we can't access
		name, err := p.Name()
		if err != nil {
			continue
		}

		cpuPercent, err := p.CPUPercent()
		if err != nil {
			continue
		}

		memPercent, err := p.MemoryPercent()
		if err != nil {
			continue
		}

		memInfo, err := p.MemoryInfo()
		if err != nil {
			continue
		}

		procInfos = append(procInfos, ProcessInfo{
			PID:          p.Pid,
			Name:         name,
			CPUPercent:   cpuPercent,
			MemPercent:   memPercent,
			MemUsedBytes: memInfo.RSS,
		})
	}

	// Sort by CPU usage descending
	sort.Slice(procInfos, func(i, j int) bool {
		return procInfos[i].CPUPercent > procInfos[j].CPUPercent
	})

	// Return top 5
	if len(procInfos) > 5 {
		return procInfos[:5]
	}
	return procInfos
}
