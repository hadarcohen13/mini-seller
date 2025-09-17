package logging

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// StandardFields defines common fields for structured logging
type StandardFields struct {
	Component  string `json:"component"`
	RequestID  string `json:"request_id,omitempty"`
	Operation  string `json:"operation,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	IPAddress  string `json:"ip_address,omitempty"`
	Duration   int64  `json:"duration_ms,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// Logger wraps logrus with structured logging standards
type Logger struct {
	*logrus.Entry
}

// NewLogger creates a new structured logger with standard fields
func NewLogger(component string) *Logger {
	entry := logrus.WithFields(logrus.Fields{
		"component": component,
		"timestamp": time.Now().Format(time.RFC3339Nano),
	})

	return &Logger{Entry: entry}
}

// NewLoggerFromContext creates a logger with context information
func NewLoggerFromContext(ctx context.Context, component string) *Logger {
	entry := logrus.WithFields(logrus.Fields{
		"component": component,
		"timestamp": time.Now().Format(time.RFC3339Nano),
	})

	// Extract request ID from context if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			entry = entry.WithField("request_id", id)
		}
	}

	// Extract OpenRTB version from context if available
	if version := ctx.Value("openrtb_version"); version != nil {
		if v, ok := version.(string); ok {
			entry = entry.WithField("openrtb_version", v)
		}
	}

	return &Logger{Entry: entry}
}

// WithRequestID adds request ID to the logger
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{Entry: l.WithField("request_id", requestID)}
}

// WithOperation adds operation name to the logger
func (l *Logger) WithOperation(operation string) *Logger {
	return &Logger{Entry: l.WithField("operation", operation)}
}

// WithDuration adds duration in milliseconds
func (l *Logger) WithDuration(duration time.Duration) *Logger {
	return &Logger{Entry: l.WithField("duration_ms", duration.Milliseconds())}
}

// WithStatusCode adds HTTP status code
func (l *Logger) WithStatusCode(code int) *Logger {
	return &Logger{Entry: l.WithField("status_code", code)}
}

// WithUserAgent adds user agent
func (l *Logger) WithUserAgent(userAgent string) *Logger {
	return &Logger{Entry: l.WithField("user_agent", userAgent)}
}

// WithIPAddress adds IP address
func (l *Logger) WithIPAddress(ip string) *Logger {
	return &Logger{Entry: l.WithField("ip_address", ip)}
}

// WithBidRequestContext adds bid request specific context
func (l *Logger) WithBidRequestContext(bidRequestID string, impressionCount int) *Logger {
	return &Logger{Entry: l.WithFields(logrus.Fields{
		"bid_request_id":   bidRequestID,
		"impression_count": impressionCount,
	})}
}

// WithBuyerContext adds buyer specific context
func (l *Logger) WithBuyerContext(buyerEndpoint string, buyerName string) *Logger {
	return &Logger{Entry: l.WithFields(logrus.Fields{
		"buyer_endpoint": buyerEndpoint,
		"buyer_name":     buyerName,
	})}
}

// WithError adds error information with appropriate context
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{Entry: l.WithField("error", err.Error())}
}

// LogLevel constants for consistent usage
const (
	// TRACE - Very detailed information, typically only of interest when diagnosing problems
	TraceLevel = logrus.TraceLevel
	// DEBUG - Detailed information, typically only of interest when diagnosing problems
	DebugLevel = logrus.DebugLevel
	// INFO - General information about application flow
	InfoLevel = logrus.InfoLevel
	// WARN - Something unexpected happened, but the application can continue
	WarnLevel = logrus.WarnLevel
	// ERROR - Something failed, but the application can continue
	ErrorLevel = logrus.ErrorLevel
	// FATAL - Something failed catastrophically, application cannot continue
	FatalLevel = logrus.FatalLevel
)

// Structured logging methods with consistent field handling

// InfoOperation logs an operation with standard fields
func (l *Logger) InfoOperation(operation, message string, duration time.Duration) {
	l.WithOperation(operation).WithDuration(duration).Info(message)
}

// WarnOperation logs a warning for an operation
func (l *Logger) WarnOperation(operation, message string, err error) {
	entry := l.WithOperation(operation)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Warn(message)
}

// ErrorOperation logs an error for an operation
func (l *Logger) ErrorOperation(operation, message string, err error) {
	l.WithOperation(operation).WithError(err).Error(message)
}

// InfoRequest logs HTTP request information
func (l *Logger) InfoRequest(method, path string, statusCode int, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"method":      method,
		"path":        path,
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
	}).Info("HTTP request processed")
}

// InfoBidRequest logs bid request processing
func (l *Logger) InfoBidRequest(bidRequestID string, impressionCount int, operation string) {
	l.WithBidRequestContext(bidRequestID, impressionCount).
		WithOperation(operation).
		Info("Bid request processing")
}

// InfoBidResponse logs bid response information
func (l *Logger) InfoBidResponse(bidRequestID string, bidCount int, totalPrice float64, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"bid_request_id": bidRequestID,
		"bid_count":      bidCount,
		"total_price":    totalPrice,
		"duration_ms":    duration.Milliseconds(),
	}).Info("Bid response generated")
}

// WarnRateLimit logs rate limiting events
func (l *Logger) WarnRateLimit(endpoint string, qps float64) {
	l.WithFields(logrus.Fields{
		"endpoint":  endpoint,
		"qps_limit": qps,
	}).Warn("Rate limit exceeded")
}

// ErrorBuyerRequest logs buyer request failures
func (l *Logger) ErrorBuyerRequest(buyerEndpoint string, attempt int, maxRetries int, err error, duration time.Duration) {
	l.WithBuyerContext(buyerEndpoint, "").
		WithFields(logrus.Fields{
			"attempt":     attempt,
			"max_retries": maxRetries,
			"duration_ms": duration.Milliseconds(),
		}).
		WithError(err).
		Error("Buyer request failed")
}

// DebugPayload logs request/response payloads (only in debug mode)
func (l *Logger) DebugPayload(direction, endpoint string, payloadSize int) {
	l.WithFields(logrus.Fields{
		"direction":    direction,
		"endpoint":     endpoint,
		"payload_size": payloadSize,
	}).Debug("Payload information")
}

// ConfigureGlobalLogger sets up global logging configuration
func ConfigureGlobalLogger(level logrus.Level, format string) {
	logrus.SetLevel(level)

	switch format {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	case "text-color":
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			ForceColors:     true,
		})
	case "compact":
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp:       false,
			FullTimestamp:          true,
			TimestampFormat:        "15:04:05",
			DisableLevelTruncation: true,
		})
	case "minimal":
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp:       true,
			DisableLevelTruncation: true,
		})
	default:
		// Default to JSON for production
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	}
}

// GetLogLevelFromString converts string to logrus level
func GetLogLevelFromString(levelStr string) logrus.Level {
	switch levelStr {
	case "trace":
		return logrus.TraceLevel
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	case "fatal":
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

// SetupLogging configures logging from environment variables
func SetupLogging() {
	// Get log level from environment (defaults to INFO)
	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = "info"
	}
	level := GetLogLevelFromString(levelStr)

	// Get log format from environment (defaults to JSON)
	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "json"
	}

	// Configure global logger
	ConfigureGlobalLogger(level, format)

	logrus.WithFields(logrus.Fields{
		"level":  level.String(),
		"format": format,
	}).Info("Logging configured")
}

// LogWithLevel logs a message at the specified level
func (l *Logger) LogWithLevel(level logrus.Level, msg string) {
	switch level {
	case logrus.DebugLevel:
		l.Debug(msg)
	case logrus.InfoLevel:
		l.Info(msg)
	case logrus.WarnLevel:
		l.Warn(msg)
	case logrus.ErrorLevel:
		l.Error(msg)
	default:
		l.Info(msg)
	}
}

// IsLevelEnabled checks if a log level is enabled
func IsLevelEnabled(level logrus.Level) bool {
	return logrus.IsLevelEnabled(level)
}
