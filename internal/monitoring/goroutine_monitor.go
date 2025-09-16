package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// GoroutineMonitor tracks and monitors goroutine usage
type GoroutineMonitor struct {
	mu               sync.RWMutex
	activeGoroutines map[string]int64
	startCounts      map[string]int64
	maxAllowed       int
	checkInterval    time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// NewGoroutineMonitor creates a new goroutine monitor
func NewGoroutineMonitor(maxGoroutines int, checkInterval time.Duration) *GoroutineMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &GoroutineMonitor{
		activeGoroutines: make(map[string]int64),
		startCounts:      make(map[string]int64),
		maxAllowed:       maxGoroutines,
		checkInterval:    checkInterval,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins monitoring goroutines
func (gm *GoroutineMonitor) Start() {
	gm.wg.Add(1)
	go gm.monitorLoop()
	logrus.Info("Goroutine monitor started")
}

// Stop stops the goroutine monitor
func (gm *GoroutineMonitor) Stop() {
	gm.cancel()
	gm.wg.Wait()
	logrus.Info("Goroutine monitor stopped")
}

// TrackGoroutineStart tracks when a goroutine starts
func (gm *GoroutineMonitor) TrackGoroutineStart(name string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.activeGoroutines[name]++
	gm.startCounts[name]++
}

// TrackGoroutineEnd tracks when a goroutine ends
func (gm *GoroutineMonitor) TrackGoroutineEnd(name string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.activeGoroutines[name] > 0 {
		gm.activeGoroutines[name]--
	}
}

// GetGoroutineStats returns current goroutine statistics
func (gm *GoroutineMonitor) GetGoroutineStats() map[string]interface{} {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	totalActive := int64(0)
	activeByType := make(map[string]int64)
	startsByType := make(map[string]int64)

	for name, count := range gm.activeGoroutines {
		totalActive += count
		activeByType[name] = count
		startsByType[name] = gm.startCounts[name]
	}

	return map[string]interface{}{
		"total_system_goroutines": runtime.NumGoroutine(),
		"tracked_active":          totalActive,
		"max_allowed":             gm.maxAllowed,
		"active_by_type":          activeByType,
		"total_starts_by_type":    startsByType,
	}
}

// CheckForLeaks checks for potential goroutine leaks
func (gm *GoroutineMonitor) CheckForLeaks() []string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	var leaks []string
	currentTotal := runtime.NumGoroutine()

	// Check if total goroutines exceed limit
	if currentTotal > gm.maxAllowed {
		leaks = append(leaks, "total goroutines exceed maximum allowed")
	}

	// Check for specific types that might be leaking
	for name, active := range gm.activeGoroutines {
		starts := gm.startCounts[name]
		if starts > 0 {
			leakRatio := float64(active) / float64(starts)
			if leakRatio > 0.8 { // 80% of started goroutines still active
				leaks = append(leaks, "potential leak in "+name+" goroutines")
			}
		}
	}

	return leaks
}

// monitorLoop runs the monitoring loop
func (gm *GoroutineMonitor) monitorLoop() {
	defer gm.wg.Done()

	ticker := time.NewTicker(gm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.performCheck()
		}
	}
}

// performCheck performs a monitoring check
func (gm *GoroutineMonitor) performCheck() {
	stats := gm.GetGoroutineStats()
	leaks := gm.CheckForLeaks()

	// Log current statistics
	logrus.WithFields(logrus.Fields{
		"total_system_goroutines": stats["total_system_goroutines"],
		"tracked_active":          stats["tracked_active"],
		"max_allowed":             stats["max_allowed"],
	}).Debug("Goroutine monitoring check")

	// Report any potential leaks
	if len(leaks) > 0 {
		logrus.WithFields(logrus.Fields{
			"leaks":           leaks,
			"goroutine_stats": stats,
		}).Warn("Potential goroutine leaks detected")
	}

	// Alert if approaching limits
	totalGoroutines := stats["total_system_goroutines"].(int)
	if float64(totalGoroutines) > float64(gm.maxAllowed)*0.9 {
		logrus.WithFields(logrus.Fields{
			"current":    totalGoroutines,
			"max":        gm.maxAllowed,
			"percentage": float64(totalGoroutines) / float64(gm.maxAllowed) * 100,
		}).Warn("Approaching goroutine limit")
	}
}
