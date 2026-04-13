// Package testutil provides lightweight helpers for writing tests against mk
// packages. It covers the three most common patterns that appear repeatedly
// across the test suite:
//
//   - FreePort: allocate a random TCP port so server tests never conflict.
//   - NewConfig: a zero-dependency core.Config stub backed by a string map,
//     with automatic type coercion for GetInt / GetBool / GetFloat64 / GetDuration.
//   - NewLogger: a core.Logger that writes to t.Log so output is visible only
//     when a test fails.
//
// Example:
//
//	func TestMyServer(t *testing.T) {
//	    addr  := testutil.FreePort(t)
//	    cfg   := testutil.NewConfig(map[string]string{"server.http.addr": addr})
//	    log   := testutil.NewLogger(t)
//	    srv, _ := httpserver.New(httpserver.WithAddr(addr), httpserver.WithLogger(log))
//	    ...
//	}
package testutil

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pemako/mk/core"
)

// FreePort allocates a random available TCP port on localhost, releases it
// immediately, and returns the address as "127.0.0.1:<port>".
// The port is not held open — there is a small window where another process
// could claim it, but in practice this is reliable enough for tests.
func FreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testutil.FreePort: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// NewConfig returns a core.Config stub backed by the provided string map.
// All Get* methods perform automatic type coercion from the stored string value:
//
//   - GetInt / GetFloat64 — parsed with strconv; returns 0 on parse error
//   - GetBool             — "true", "1", "yes" (case-insensitive) → true
//   - GetDuration         — parsed with time.ParseDuration; returns 0 on error
//   - GetStringSlice      — splits on comma
//   - Scan / Watch        — no-ops; always return nil
func NewConfig(m map[string]string) core.Config {
	return &stubConfig{m: m}
}

// NewLogger returns a core.Logger that writes all log entries to t.Log.
// Output is only shown by `go test` when the test fails or -v is used,
// keeping successful test output clean.
func NewLogger(t *testing.T) core.Logger {
	return &testLogger{t: t}
}

// --- stubConfig ---

type stubConfig struct{ m map[string]string }

func (s *stubConfig) Get(key string) any          { return s.m[key] }
func (s *stubConfig) GetString(key string) string { return s.m[key] }

func (s *stubConfig) GetInt(key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s.m[key]))
	return n
}

func (s *stubConfig) GetBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(s.m[key]))
	return v == "true" || v == "1" || v == "yes"
}

func (s *stubConfig) GetFloat64(key string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s.m[key]), 64)
	return f
}

func (s *stubConfig) GetDuration(key string) time.Duration {
	d, _ := time.ParseDuration(strings.TrimSpace(s.m[key]))
	return d
}

func (s *stubConfig) GetStringSlice(key string) []string {
	v := strings.TrimSpace(s.m[key])
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func (s *stubConfig) Scan(key string, dest any) error      { return nil }
func (s *stubConfig) Watch(key string, fn func(any)) error { return nil }

// --- testLogger ---

type testLogger struct {
	t      *testing.T
	fields []core.Field
}

func (l *testLogger) log(level, msg string, fields []core.Field) {
	l.t.Helper()
	all := append(l.fields, fields...) //nolint:gocritic
	parts := make([]string, 0, len(all)+2)
	parts = append(parts, level, msg)
	for _, f := range all {
		parts = append(parts, f.Key+"="+stringify(f.Value))
	}
	l.t.Log(strings.Join(parts, " "))
}

func (l *testLogger) Debug(_ context.Context, msg string, fields ...core.Field) {
	l.log("DEBUG", msg, fields)
}
func (l *testLogger) Info(_ context.Context, msg string, fields ...core.Field) {
	l.log("INFO", msg, fields)
}
func (l *testLogger) Warn(_ context.Context, msg string, fields ...core.Field) {
	l.log("WARN", msg, fields)
}
func (l *testLogger) Error(_ context.Context, msg string, fields ...core.Field) {
	l.log("ERROR", msg, fields)
}
func (l *testLogger) With(fields ...core.Field) core.Logger {
	combined := make([]core.Field, len(l.fields)+len(fields))
	copy(combined, l.fields)
	copy(combined[len(l.fields):], fields)
	return &testLogger{t: l.t, fields: combined}
}

func stringify(v any) string {
	if v == nil {
		return "<nil>"
	}
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}
