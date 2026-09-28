// Redactor: removes tokens and titles from Plex errors and logs
// (docs/security.md).

package plex

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// tokenPattern matches an X-Plex-Token carried as a query parameter or a
// header line, whatever the value.
var tokenPattern = regexp.MustCompile(`(?i)(x-plex-token)(=|:\s*|"\s*:\s*")([^&\s"'\]]+)`)

// Redactor strips known secrets and item titles from text destined for logs,
// diagnostics, or state messages. Secrets are registered when learned (the
// account token); titles when items are fetched. Safe for concurrent use.
type Redactor struct {
	mu      sync.RWMutex
	secrets []string
	titles  []string
}

// NewRedactor returns an empty redactor; the X-Plex-Token pattern is always
// applied even before any secret is registered.
func NewRedactor() *Redactor { return &Redactor{} }

// AddSecret registers a secret value to replace with "[token]". Empty and
// very short values are ignored so common substrings are not blanked.
func (r *Redactor) AddSecret(s string) {
	if len(s) < 4 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets = insertLongestFirst(r.secrets, s)
}

// AddTitle registers a private content title to replace with "[title]".
func (r *Redactor) AddTitle(s string) {
	if len(strings.TrimSpace(s)) < 2 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.titles = insertLongestFirst(r.titles, s)
}

func insertLongestFirst(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	list = append(list, s)
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	return list
}

// Redact returns s with every registered secret, X-Plex-Token value, and
// registered title replaced.
func (r *Redactor) Redact(s string) string {
	s = tokenPattern.ReplaceAllString(s, "${1}${2}[token]")
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sec := range r.secrets {
		s = strings.ReplaceAll(s, sec, "[token]")
	}
	for _, title := range r.titles {
		s = strings.ReplaceAll(s, title, "[title]")
	}
	return s
}

// Error returns an error whose text is the redacted message of err. The
// result satisfies errors.Is/errors.As against err's chain but does not
// unwrap to it, so the unredacted text cannot be recovered by formatting.
func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{msg: r.Redact(err.Error()), cause: err}
}

type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }

func (e *redactedError) Is(target error) bool { return errors.Is(e.cause, target) }

func (e *redactedError) As(target any) bool { return errors.As(e.cause, target) }

// Handler wraps h so every record's message and string, error, and group
// attributes pass through Redact before h sees them.
func (r *Redactor) Handler(h slog.Handler) slog.Handler { return &redactHandler{r: r, h: h} }

type redactHandler struct {
	r *Redactor
	h slog.Handler
}

func (h *redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.Redact(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(a))
		return true
	})
	return h.h.Handle(ctx, out)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = h.attr(a)
	}
	return &redactHandler{r: h.r, h: h.h.WithAttrs(out)}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{r: h.r, h: h.h.WithGroup(name)}
}

func (h *redactHandler) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.r.Redact(v.String()))
	case slog.KindGroup:
		members := v.Group()
		out := make([]any, 0, len(members))
		for _, m := range members {
			out = append(out, h.attr(m))
		}
		return slog.Group(a.Key, out...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, h.r.Redact(err.Error()))
		}
		return slog.String(a.Key, h.r.Redact(v.String()))
	}
	return slog.Attr{Key: a.Key, Value: v}
}
