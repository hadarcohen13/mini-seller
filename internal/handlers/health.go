package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/hadarco13/mini-seller/internal/metrics"
	"github.com/hadarco13/mini-seller/internal/redis"
	"github.com/sirupsen/logrus"
)

// HealthStatus represents the application health status
type HealthStatus struct {
	Status         string                      `json:"status"`
	GoroutineStats GoroutineHealthStats        `json:"goroutine_stats"`
	RequestStats   map[string]int64            `json:"request_stats"`
	Dependencies   map[string]DependencyStatus `json:"dependencies"`
	Warnings       []string                    `json:"warnings,omitempty"`
}

// DependencyStatus represents the status of a dependency
type DependencyStatus struct {
	Status       string        `json:"status"`
	ResponseTime time.Duration `json:"response_time_ms"`
	Error        string        `json:"error,omitempty"`
	LastChecked  time.Time     `json:"last_checked"`
}

// GoroutineHealthStats represents goroutine health metrics
type GoroutineHealthStats struct {
	Total      int         `json:"total"`
	MaxAllowed int         `json:"max_allowed"`
	Percentage float64     `json:"percentage"`
	MemStats   MemoryStats `json:"memory_stats"`
}

// MemoryStats represents memory usage statistics
type MemoryStats struct {
	AllocMB      uint64 `json:"alloc_mb"`
	TotalAllocMB uint64 `json:"total_alloc_mb"`
	SysMB        uint64 `json:"sys_mb"`
	NumGC        uint32 `json:"num_gc"`
}

// HealthHandler provides health check endpoint with goroutine monitoring
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	status := checkApplicationHealth()

	// Set appropriate HTTP status code
	httpStatus := http.StatusOK
	if status.Status == "warning" {
		httpStatus = http.StatusPartialContent
	} else if status.Status == "critical" {
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		logrus.WithError(err).Error("Failed to encode health status")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}

	// Log health check results
	logrus.WithFields(logrus.Fields{
		"status":           status.Status,
		"total_goroutines": status.GoroutineStats.Total,
		"memory_alloc_mb":  status.GoroutineStats.MemStats.AllocMB,
		"warnings":         len(status.Warnings),
	}).Debug("Health check performed")
}

// checkApplicationHealth performs comprehensive health checks
func checkApplicationHealth() HealthStatus {
	var warnings []string
	status := "healthy"

	// Check dependencies
	dependencies := checkDependencies()

	// Check if any dependencies are unhealthy
	for depName, depStatus := range dependencies {
		if depStatus.Status == "unhealthy" {
			status = "critical"
			warnings = append(warnings, depName+" dependency is unhealthy")
		} else if depStatus.Status == "degraded" {
			if status == "healthy" {
				status = "warning"
			}
			warnings = append(warnings, depName+" dependency is degraded")
		}
	}

	// Get goroutine stats
	totalGoroutines := runtime.NumGoroutine()
	maxAllowed := 1000 // Configurable limit

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	goroutineStats := GoroutineHealthStats{
		Total:      totalGoroutines,
		MaxAllowed: maxAllowed,
		Percentage: float64(totalGoroutines) / float64(maxAllowed) * 100,
		MemStats: MemoryStats{
			AllocMB:      bToMb(memStats.Alloc),
			TotalAllocMB: bToMb(memStats.TotalAlloc),
			SysMB:        bToMb(memStats.Sys),
			NumGC:        memStats.NumGC,
		},
	}

	// Check goroutine count
	if goroutineStats.Percentage > 90 {
		status = "critical"
		warnings = append(warnings, "goroutine count exceeds 90% of maximum")
	} else if goroutineStats.Percentage > 70 {
		if status == "healthy" {
			status = "warning"
		}
		warnings = append(warnings, "goroutine count exceeds 70% of maximum")
	}

	// Check memory usage
	if goroutineStats.MemStats.AllocMB > 500 {
		if status != "critical" {
			status = "warning"
		}
		warnings = append(warnings, "high memory allocation detected")
	}

	// Get request stats
	requestStats := GetRequestStats()

	// Check request failure rate
	if requestStats["success_rate"] < 90 && requestStats["total_requests"] > 10 {
		if status != "critical" {
			status = "warning"
		}
		warnings = append(warnings, "high request failure rate detected")
	}

	return HealthStatus{
		Status:         status,
		GoroutineStats: goroutineStats,
		RequestStats:   requestStats,
		Dependencies:   dependencies,
		Warnings:       warnings,
	}
}

// bToMb converts bytes to megabytes
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// ForceGC triggers garbage collection and reports stats
func ForceGCHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var beforeStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)
	beforeGoroutines := runtime.NumGoroutine()

	// Force garbage collection
	runtime.GC()
	runtime.GC() // Run twice for thoroughness

	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)
	afterGoroutines := runtime.NumGoroutine()

	result := map[string]interface{}{
		"before": map[string]interface{}{
			"goroutines": beforeGoroutines,
			"alloc_mb":   bToMb(beforeStats.Alloc),
			"sys_mb":     bToMb(beforeStats.Sys),
		},
		"after": map[string]interface{}{
			"goroutines": afterGoroutines,
			"alloc_mb":   bToMb(afterStats.Alloc),
			"sys_mb":     bToMb(afterStats.Sys),
		},
		"freed_mb": bToMb(beforeStats.Alloc - afterStats.Alloc),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)

	logrus.WithFields(logrus.Fields{
		"before_goroutines": beforeGoroutines,
		"after_goroutines":  afterGoroutines,
		"freed_mb":          result["freed_mb"],
	}).Info("Forced garbage collection completed")
}

// MetricsHandler provides application metrics endpoint
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	stats := metrics.GetGlobalStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logrus.WithError(err).Error("Failed to encode metrics")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logrus.Debug("Metrics endpoint accessed")
}

// PerformanceHandler provides performance monitoring endpoint
func PerformanceHandler(w http.ResponseWriter, r *http.Request) {
	performanceStats := metrics.GetPerformanceStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(performanceStats); err != nil {
		logrus.WithError(err).Error("Failed to encode performance stats")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logrus.Debug("Performance endpoint accessed")
}

// checkDependencies checks the health of all dependencies
func checkDependencies() map[string]DependencyStatus {
	dependencies := make(map[string]DependencyStatus)

	// Check Redis
	dependencies["redis"] = checkRedisHealth()

	return dependencies
}

// checkRedisHealth checks Redis connection health
func checkRedisHealth() DependencyStatus {
	start := time.Now()
	status := DependencyStatus{
		LastChecked: start,
	}

	// Get Redis client
	client := redis.GetClient()
	if client == nil {
		status.Status = "unhealthy"
		status.Error = "Redis client not initialized"
		status.ResponseTime = time.Since(start)
		return status
	}

	// Test Redis connection with ping
	cfg := config.GetConfig()
	timeout := time.Duration(cfg.Server.HealthCheckTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	status.ResponseTime = time.Since(start)

	if err != nil {
		if status.ResponseTime > 2*time.Second {
			status.Status = "degraded"
			status.Error = "Redis response time too slow: " + err.Error()
		} else {
			status.Status = "unhealthy"
			status.Error = "Redis ping failed: " + err.Error()
		}
	} else {
		if status.ResponseTime > 1*time.Second {
			status.Status = "degraded"
			status.Error = "Redis response time slow"
		} else {
			status.Status = "healthy"
		}
	}

	return status
}
