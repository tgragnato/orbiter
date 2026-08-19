package tui

import (
	"context"
	"log/slog"
	"time"
)

const logChannelBufferSize = 256

// LogEntry is a single captured slog record.
type LogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

// LogChannel streams log entries into the TUI.
type LogChannel chan LogEntry

// NewLogChannel creates a buffered channel for log entries.
func NewLogChannel() LogChannel {
	return make(LogChannel, logChannelBufferSize)
}

// TUIHandler is a slog.Handler that forwards records to a LogChannel.
// Drops entries silently when the buffer is full so callers never block.
type TUIHandler struct {
	ch    LogChannel
	attrs []slog.Attr
	group string
}

// NewTUIHandler returns a handler that sends records to ch.
func NewTUIHandler(ch LogChannel) *TUIHandler {
	return &TUIHandler{
		ch:    ch,
		attrs: nil,
		group: "",
	}
}

// Enabled always returns true — all levels are captured for the log view.
func (h *TUIHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle forwards the record to the log channel, dropping it when the buffer is full.
func (h *TUIHandler) Handle(_ context.Context, record slog.Record) error {
	entry := LogEntry{
		Time:    record.Time,
		Level:   record.Level,
		Message: record.Message,
		Attrs:   nil,
	}
	record.Attrs(func(attr slog.Attr) bool {
		entry.Attrs = append(entry.Attrs, attr)

		return true
	})

	select {
	case h.ch <- entry:
	default:
	}

	return nil
}

// WithAttrs returns a new handler that prepends attrs to every record.
func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &TUIHandler{
		ch:    h.ch,
		group: h.group,
		attrs: nil,
	}
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)

	return next
}

// WithGroup returns a new handler scoped to the given group name.
func (h *TUIHandler) WithGroup(name string) slog.Handler {
	return &TUIHandler{ch: h.ch, attrs: h.attrs, group: name}
}
