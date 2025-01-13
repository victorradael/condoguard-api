package performance

import (
    "context"
    "runtime"
    "time"
    "github.com/victorradael/condoguard/internal/logger"
    "github.com/shirou/gopsutil/v3/cpu"
    "github.com/shirou/gopsutil/v3/mem"
)

type PerformanceStats struct {
    Timestamp     time.Time
    GoRoutines    int
    MemStats      runtime.MemStats
    CPUUsage      float64
    MemoryUsage   float64
    GCPauseNs     uint64
    HeapObjects   uint64
    ResponseTimes map[string]time.Duration
}

type PerformanceMonitor struct {
    stats         chan PerformanceStats
    collectPeriod time.Duration
}

func NewPerformanceMonitor(collectPeriod time.Duration) *PerformanceMonitor {
    return &PerformanceMonitor{
        stats:         make(chan PerformanceStats, 100),
        collectPeriod: collectPeriod,
    }
}

func (pm *PerformanceMonitor) Start(ctx context.Context) {
    go pm.collect(ctx)
    go pm.report(ctx)
}

func (pm *PerformanceMonitor) collect(ctx context.Context) {
    ticker := time.NewTicker(pm.collectPeriod)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            stats := PerformanceStats{
                Timestamp:  time.Now(),
                GoRoutines: runtime.NumGoroutine(),
            }

            // Collect memory stats
            runtime.ReadMemStats(&stats.MemStats)

            // Collect CPU usage
            if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
                stats.CPUUsage = cpuPercent[0]
            }

            // Collect memory usage
            if vmStat, err := mem.VirtualMemory(); err == nil {
                stats.MemoryUsage = vmStat.UsedPercent
            }

            pm.stats <- stats
        }
    }
}

func (pm *PerformanceMonitor) report(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case stats := <-pm.stats:
            logger.Info("Performance Stats", logger.Fields{
                "timestamp":    stats.Timestamp,
                "goroutines":   stats.GoRoutines,
                "heap_objects": stats.MemStats.HeapObjects,
                "heap_alloc":   stats.MemStats.HeapAlloc,
                "cpu_usage":    stats.CPUUsage,
                "memory_usage": stats.MemoryUsage,
            })
        }
    }
} 