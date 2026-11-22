// Package applog is authama's own log: what ran and how it went - commands,
// web requests and actions, jobs, watcher events and sign-ins - as one JSON
// line each in authama.log, in the state home's log directory.
//
// It is apart from the audit trail (audit.log, which feeds the history) and
// holds no secret: every record passes Redact, which blanks any attribute
// named like a secret (token, secret, passphrase, pin, password, key
// material, cookies, the control key) and any value that looks like one (a
// private key block, a sign-in link's token, a GitHub or age secret). Times
// are written in UTC; what shows them converts.
//
// The file is rotated at a size, and rotated files older than the retention
// are removed. Several processes may append at once (the web UI and a
// command); each record is one write, so lines do not interleave.
package applog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileName is the live log in the log directory; rotated ones are
// authama-<UTC time>.log beside it.
const FileName = "authama.log"

// Defaults for the log settings.
const (
	DefaultLevel         = "info"
	DefaultRetentionDays = 14
	DefaultMaxSizeMB     = 5
)

// Levels are the names log.level takes, least severe first.
var Levels = []string{"debug", "info", "warn", "error"}

// ValidLevel reports whether s names a level: debug, info, warn or error.
func ValidLevel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "info", "warn", "warning", "error":
		return true
	}
	return false
}

// ParseLevel reads a level name; anything else is info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}

// Config is where and how much to log.
type Config struct {
	Dir           string
	Level         slog.Level
	RetentionDays int
	MaxSizeMB     int
}

var (
	mu     sync.Mutex
	logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	active Config
)

// Configure points the log at cfg.Dir; until it is called, or with no Dir,
// nothing is written.
func Configure(cfg Config) {
	if cfg.Dir == "" {
		mu.Lock()
		logger, active = slog.New(slog.NewJSONHandler(io.Discard, nil)), Config{}
		mu.Unlock()
		return
	}
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = DefaultRetentionDays
	}
	if cfg.MaxSizeMB < 1 {
		cfg.MaxSizeMB = DefaultMaxSizeMB
	}
	w := &rotating{cfg: cfg}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.Level, ReplaceAttr: replace})
	mu.Lock()
	logger, active = slog.New(h), cfg
	mu.Unlock()
}

// L is the log; a discarding one until Configure.
func L() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Active is the configuration in use.
func Active() Config {
	mu.Lock()
	defer mu.Unlock()
	return active
}

// Info, Warn, Error and Debug write one record.
func Info(msg string, args ...any)  { L().Info(msg, args...) }
func Warn(msg string, args ...any)  { L().Warn(msg, args...) }
func Error(msg string, args ...any) { L().Error(msg, args...) }
func Debug(msg string, args ...any) { L().Debug(msg, args...) }

// Enabled reports whether level would be written.
func Enabled(level slog.Level) bool { return L().Enabled(context.Background(), level) }

// secretKey matches attribute names whose values are never logged.
var secretKey = regexp.MustCompile(`(?i)(token|secret|passphrase|password|passwd|pin$|^pin|private|cookie|authorization|control|credential|api_?key|signature)`)

// secretValue matches values that look like a secret wherever they appear.
var secretValue = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`([?&](token|challenge|caller|proof)=)[^&\s"]+`),
	regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,})`),
	regexp.MustCompile(`AGE-SECRET-KEY-1[0-9A-Z]{20,}`),
	regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/=-]{8,}`),
}

// Redacted replaces whatever Redact removes.
const Redacted = "[redacted]"

// Redact blanks what looks like a secret in s.
func Redact(s string) string {
	for i, re := range secretValue {
		switch i {
		case 1:
			s = re.ReplaceAllString(s, "${1}"+Redacted)
		case 4:
			s = re.ReplaceAllString(s, "${1} "+Redacted)
		default:
			s = re.ReplaceAllString(s, Redacted)
		}
	}
	return s
}

func replace(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && (a.Key == slog.TimeKey || a.Key == slog.LevelKey || a.Key == slog.MessageKey) {
		if a.Key == slog.TimeKey {
			return slog.String(slog.TimeKey, a.Value.Time().UTC().Format(time.RFC3339Nano))
		}
		if a.Key == slog.MessageKey {
			return slog.String(a.Key, Redact(a.Value.String()))
		}
		return a
	}
	if secretKey.MatchString(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Redact(a.Value.String()))
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Redact(err.Error()))
		}
		if ss, ok := a.Value.Any().([]string); ok {
			out := make([]string, len(ss))
			for i, v := range ss {
				out[i] = Redact(v)
			}
			return slog.Any(a.Key, redactArgs(out))
		}
		return slog.String(a.Key, Redact(fmt.Sprint(a.Value.Any())))
	}
	return a
}

// redactArgs blanks a flag's value when the flag is named like a secret
// (--passphrase x, --pin=1234), for a command line.
func redactArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !secretKey.MatchString(name) || strings.HasSuffix(name, "-file") {
			continue
		}
		if hasValue {
			out[i] = "--" + name + "=" + Redacted
		} else if i+1 < len(out) && !strings.HasPrefix(out[i+1], "-") {
			out[i+1] = Redacted
		}
	}
	return out
}

// Args is a command line for a record, with secret flag values blanked.
func Args(args []string) []string { return redactArgs(args) }

// rotating appends to Dir/authama.log, rotating it past MaxSizeMB.
type rotating struct {
	cfg Config
	mu  sync.Mutex
}

func (w *rotating) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(w.cfg.Dir, 0o700); err != nil {
		return 0, err
	}
	path := filepath.Join(w.cfg.Dir, FileName)
	if fi, err := os.Stat(path); err == nil && fi.Size()+int64(len(p)) > int64(w.cfg.MaxSizeMB)<<20 {
		rotated := filepath.Join(w.cfg.Dir, "authama-"+time.Now().UTC().Format("20060102T150405.000")+".log")
		if os.Rename(path, rotated) == nil {
			Prune(w.cfg.Dir, w.cfg.RetentionDays, time.Now())
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	return f.Write(p)
}

// Files is the log files in dir, oldest first, the live one last.
func Files(dir string) []string {
	rotated, _ := filepath.Glob(filepath.Join(dir, "authama-*.log"))
	sort.Strings(rotated)
	live := filepath.Join(dir, FileName)
	if _, err := os.Stat(live); err == nil {
		rotated = append(rotated, live)
	}
	return rotated
}

// Prune removes rotated logs last written more than days ago.
func Prune(dir string, days int, now time.Time) int {
	n := 0
	rotated, _ := filepath.Glob(filepath.Join(dir, "authama-*.log"))
	for _, f := range rotated {
		if fi, err := os.Stat(f); err == nil && now.Sub(fi.ModTime()) > time.Duration(days)*24*time.Hour {
			if os.Remove(f) == nil {
				n++
			}
		}
	}
	return n
}

// Entry is one record read back.
type Entry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
	Raw   string         `json:"-"`
}

// Filter picks records: at least Level, at or after Since, and containing
// Grep (any case) anywhere in the line.
type Filter struct {
	Level slog.Level
	Since time.Time
	Grep  string
	Limit int // the newest Limit; 0 is all
}

// Match reports whether e passes f.
func (f Filter) Match(e Entry) bool {
	if ParseLevel(e.Level) < f.Level {
		return false
	}
	if !f.Since.IsZero() && e.Time.Before(f.Since) {
		return false
	}
	return f.Grep == "" || strings.Contains(strings.ToLower(e.Raw), strings.ToLower(f.Grep))
}

// Parse reads one line.
func Parse(line string) (Entry, bool) {
	var m map[string]any
	if json.Unmarshal([]byte(line), &m) != nil {
		return Entry{}, false
	}
	e := Entry{Raw: line, Attrs: map[string]any{}}
	for k, v := range m {
		switch k {
		case slog.TimeKey:
			s, _ := v.(string)
			e.Time, _ = time.Parse(time.RFC3339Nano, s)
		case slog.LevelKey:
			e.Level, _ = v.(string)
		case slog.MessageKey:
			e.Msg, _ = v.(string)
		default:
			e.Attrs[k] = v
		}
	}
	return e, true
}

// Read is every record in dir that f picks, oldest first.
func Read(dir string, f Filter) ([]Entry, error) {
	var out []Entry
	for _, path := range Files(dir) {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(file)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			if e, ok := Parse(sc.Text()); ok && f.Match(e) {
				out = append(out, e)
			}
		}
		_ = file.Close()
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out, nil
}

// Size is how long the live log is now: where a follow starts.
func Size(dir string) int64 {
	fi, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Tail returns the records written to the live log after offset, and the
// offset to read from next; a log that was rotated starts again at 0.
func Tail(dir string, offset int64, f Filter) ([]Entry, int64, error) {
	path := filepath.Join(dir, FileName)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, 0, nil
	}
	if fi.Size() < offset {
		offset = 0
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	b, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		return nil, offset, err
	}
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return nil, offset, nil
	}
	var out []Entry
	for _, line := range strings.Split(string(b[:end]), "\n") {
		if e, ok := Parse(line); ok && f.Match(e) {
			out = append(out, e)
		}
	}
	return out, offset + int64(end) + 1, nil
}
