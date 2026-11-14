package core

import (
	"context"
	"log/slog"
)

// discardHandler drops every log record. It is the default logger so a
// library caller that does not set one sees no output.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool { return false }

func (discardHandler) Handle(context.Context, slog.Record) error { return nil }

func (discardHandler) WithAttrs([]slog.Attr) slog.Handler { return discardHandler{} }

func (discardHandler) WithGroup(string) slog.Handler { return discardHandler{} }

func discardLogger() *slog.Logger {
	return slog.New(discardHandler{})
}
