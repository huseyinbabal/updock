// Package logger provides structured, high-performance logging for Updock.
//
// It wraps zerolog with a simplified API and supports:
//   - Structured key-value logging
//   - Async writing via diode writer for non-blocking I/O
//   - Console (human-readable) and JSON output formats
//   - Dynamic log level changes at runtime
//
// # Usage
//
//	logger.Setup("debug", false) // debug level, console format
//	logger.Info().Str("container", "nginx").Msg("checking for updates")
//	logger.Error().Err(err).Str("container", "nginx").Msg("update failed")
//
// # Async Writing
//
// By default, logs are written asynchronously using a ring buffer. This
// prevents slow I/O (e.g. piping to a file) from blocking the update loop.
// Up to 1000 messages can be buffered; older messages are dropped under pressure.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
)

// global is the package-level logger instance.
var global zerolog.Logger

func init() {
	// Default: info level, console output, async
	Setup("info", false)
}

// Setup initializes the global logger with the given level and format.
// If jsonFormat is true, output is JSON (for machine consumption).
// Otherwise, output is human-readable console format with colors.
func Setup(level string, jsonFormat bool) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var writer io.Writer
	if jsonFormat {
		// Async JSON writer
		writer = diode.NewWriter(os.Stdout, 1000, 10*time.Millisecond, func(missed int) {
			// silently drop missed messages under back-pressure
		})
	} else {
		// Console writer (human-readable) wrapped in async diode
		cw := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
		writer = diode.NewWriter(cw, 1000, 10*time.Millisecond, func(missed int) {})
	}

	global = zerolog.New(writer).With().Timestamp().Logger()
}

// Debug starts a new debug-level log event.
func Debug() *zerolog.Event { return global.Debug() }

// Info starts a new info-level log event.
func Info() *zerolog.Event { return global.Info() }

// Warn starts a new warn-level log event.
func Warn() *zerolog.Event { return global.Warn() }

// Error starts a new error-level log event.
func Error() *zerolog.Event { return global.Error() }

// Fatal starts a new fatal-level log event. The process exits after logging.
func Fatal() *zerolog.Event { return global.Fatal() }

// With returns a child logger with the given key-value context.
func With() zerolog.Context { return global.With() }

// Get returns the underlying zerolog.Logger for advanced usage.
func Get() zerolog.Logger { return global }
