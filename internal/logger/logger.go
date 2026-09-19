package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	waLog "go.mau.fi/whatsmeow/util/log"
)

func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

var (
	phoneRe    = regexp.MustCompile(`\b\d{7,15}\b`)
	qrLineRe   = regexp.MustCompile(`(?i)qr[\s_-]?code`)
	groupJIDRe = regexp.MustCompile(`\b\d{7,15}-\d{7,15}@g\.us\b`)
)

type waLogAdapter struct {
	l     *slog.Logger
	level slog.Level
	mod   string
}

func WhatsAppLogger(l *slog.Logger, module string) waLog.Logger {
	return &waLogAdapter{
		l:     l.With("component", "whatsmeow", "module", module),
		level: slog.LevelDebug,
		mod:   module,
	}
}

func (a *waLogAdapter) sanitize(msg string) string {
	if strings.Contains(a.mod, "QRChannel") || qrLineRe.MatchString(msg) {
		return "[redacted QR payload by whatsapp-gateway privacy filter]"
	}
	msg = groupJIDRe.ReplaceAllString(msg, "[group]")
	return MaskPhoneNumbers(msg)
}

func (a *waLogAdapter) log(level slog.Level, format string, args ...any) {
	if !a.l.Enabled(context.Background(), level) {
		return
	}
	a.l.Log(context.Background(), level, a.sanitize(fmt.Sprintf(format, args...)))
}

func (a *waLogAdapter) Debugf(msg string, args ...any) { a.log(slog.LevelDebug, msg, args...) }
func (a *waLogAdapter) Infof(msg string, args ...any)  { a.log(slog.LevelInfo, msg, args...) }
func (a *waLogAdapter) Warnf(msg string, args ...any)  { a.log(slog.LevelWarn, msg, args...) }
func (a *waLogAdapter) Errorf(msg string, args ...any) { a.log(slog.LevelError, msg, args...) }
func (a *waLogAdapter) Sub(module string) waLog.Logger {
	return &waLogAdapter{l: a.l, level: a.level, mod: a.mod + "/" + module}
}

var _ waLog.Logger = (*waLogAdapter)(nil)

// MaskPhoneNumbers keeps only the first four and last two digits of any digit
// run, enough to correlate log lines without exposing a full phone number.
func MaskPhoneNumbers(msg string) string {
	return phoneRe.ReplaceAllStringFunc(msg, func(s string) string {
		if len(s) <= 6 {
			return s
		}
		return s[:4] + strings.Repeat("*", len(s)-6) + s[len(s)-2:]
	})
}
