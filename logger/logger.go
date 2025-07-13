package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
)

// Color constants for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorGray   = "\033[37m"
	ColorCyan   = "\033[36m"
	ColorGreen  = "\033[32m"
	ColorPurple = "\033[35m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
)

// PrettyHandler is a slog.Handler that outputs colorized, human-readable logs
type PrettyHandler struct {
	opts   *slog.HandlerOptions
	output io.Writer
	attrs  []slog.Attr
	groups []string
}

// NewPrettyHandler creates a new PrettyHandler
func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	}
	return &PrettyHandler{
		opts:   opts,
		output: w,
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *PrettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

// Handle formats and outputs a log record
func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	// Format timestamp
	timestamp := r.Time.Format("15:04:05.000")

	// Get level with color
	levelStr := h.formatLevel(r.Level)

	// Get caller info
	caller := h.getCaller(r)

	// Build the log line
	var buf strings.Builder

	// Time + Level
	buf.WriteString(fmt.Sprintf("%s%s%s %s",
		ColorDim, timestamp, ColorReset, levelStr))

	// Caller info
	if caller != "" {
		buf.WriteString(fmt.Sprintf(" %s%s%s", ColorDim, caller, ColorReset))
	}

	// Message
	buf.WriteString(fmt.Sprintf(" %s%s%s",
		ColorBold, r.Message, ColorReset))

	// Attributes - inline format
	if r.NumAttrs() > 0 || len(h.attrs) > 0 {
		buf.WriteString(" ")
		h.appendAttrs(&buf, r)
	}

	buf.WriteString("\n")

	_, err := h.output.Write([]byte(buf.String()))
	return err
}

// WithAttrs returns a new handler with additional attributes
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)
	newAttrs = append(newAttrs, attrs...)

	return &PrettyHandler{
		opts:   h.opts,
		output: h.output,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with an additional group
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, 0, len(h.groups)+1)
	newGroups = append(newGroups, h.groups...)
	newGroups = append(newGroups, name)

	return &PrettyHandler{
		opts:   h.opts,
		output: h.output,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// formatLevel returns a colorized level string
func (h *PrettyHandler) formatLevel(level slog.Level) string {
	var color string
	var levelStr string

	switch {
	case level >= slog.LevelError:
		color = ColorRed
		levelStr = "ERROR"
	case level >= slog.LevelWarn:
		color = ColorYellow
		levelStr = "WARN "
	case level >= slog.LevelInfo:
		color = ColorGreen
		levelStr = "INFO "
	default:
		color = ColorCyan
		levelStr = "DEBUG"
	}

	return fmt.Sprintf("%s[%s]%s", color, levelStr, ColorReset)
}

// getCaller returns formatted caller information
func (h *PrettyHandler) getCaller(r slog.Record) string {
	if !h.opts.AddSource || r.PC == 0 {
		return ""
	}

	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()

	// Extract just the filename and line number
	parts := strings.Split(f.File, "/")
	filename := parts[len(parts)-1]

	return fmt.Sprintf("%s:%d", filename, f.Line)
}

// appendAttrs formats and appends attributes to the buffer
func (h *PrettyHandler) appendAttrs(buf *strings.Builder, r slog.Record) {
	var attrs []string

	// Add handler-level attributes first
	for _, attr := range h.attrs {
		if attrStr := h.formatAttr(attr, ""); attrStr != "" {
			attrs = append(attrs, attrStr)
		}
	}

	// Add record-level attributes
	r.Attrs(func(attr slog.Attr) bool {
		if attrStr := h.formatAttr(attr, ""); attrStr != "" {
			attrs = append(attrs, attrStr)
		}
		return true
	})

	// Join all attributes with ", "
	if len(attrs) > 0 {
		buf.WriteString(strings.Join(attrs, " "))
	}
}

// formatAttr formats a single attribute as key=value
func (h *PrettyHandler) formatAttr(attr slog.Attr, prefix string) string {
	if attr.Equal(slog.Attr{}) {
		return ""
	}

	key := attr.Key
	if prefix != "" {
		key = prefix + "." + key
	}

	// Add group prefixes
	for _, group := range h.groups {
		key = group + "." + key
	}

	value := attr.Value.String()

	// Format as key=value with colors
	return fmt.Sprintf("%s%s%s=%s%s%s",
		ColorPurple, key, ColorReset,
		ColorCyan, value, ColorReset)
}

// Global logger instance
var Logger *slog.Logger

// init initializes the global logger
func init() {
	InitLogger()
}

// InitLogger initializes the global logger with pretty formatting
func InitLogger() {
	opts := &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}

	handler := NewPrettyHandler(os.Stderr, opts)
	Logger = slog.New(handler)

	// Set as default logger
	slog.SetDefault(Logger)
}

// InitLoggerWithLevel initializes the logger with a specific level
func InitLoggerWithLevel(level slog.Level) {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	handler := NewPrettyHandler(os.Stderr, opts)
	Logger = slog.New(handler)

	// Set as default logger
	slog.SetDefault(Logger)
}

// InitLoggerWithOutput initializes the logger with custom output
func InitLoggerWithOutput(w io.Writer, level slog.Level) {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	handler := NewPrettyHandler(w, opts)
	Logger = slog.New(handler)

	// Set as default logger
	slog.SetDefault(Logger)
}

// Convenience functions for logging
func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}

// Convenience functions with context
func DebugContext(ctx context.Context, msg string, args ...any) {
	Logger.DebugContext(ctx, msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Logger.InfoContext(ctx, msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Logger.WarnContext(ctx, msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Logger.ErrorContext(ctx, msg, args...)
}

// With returns a logger with additional attributes
func With(args ...any) *slog.Logger {
	return Logger.With(args...)
}

// WithGroup returns a logger with a group
func WithGroup(name string) *slog.Logger {
	return Logger.WithGroup(name)
}
