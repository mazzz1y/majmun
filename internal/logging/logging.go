package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"majmun/internal/ctxutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

var logger *slog.Logger

func init() {
	SetLevelAndFormat("debug", "text")
}

func SetLevelAndFormat(l, f string) {
	var level slog.Level
	levelLower := strings.ToLower(l)
	switch levelLower {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
		if logger != nil {
			logger.Warn("invalid log level, defaulting to 'info'")
		}
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	formatLower := strings.ToLower(f)
	switch formatLower {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
		if logger != nil {
			logger.Warn("invalid log format, defaulting to 'text'")
		}
	}

	logger = slog.New(handler)
}

func Info(ctx context.Context, msg string, args ...any) {
	if ctxutil.ChannelHidden(ctx) {
		return
	}
	log(ctx, slog.LevelInfo, msg, args...)
}

func Error(ctx context.Context, err error, msg string, args ...any) {
	if err != nil {
		args = append(args, "error", err)
	}

	level := slog.LevelError
	if errors.Is(err, context.Canceled) || errors.Is(err, syscall.EPIPE) {
		level = slog.LevelDebug
	}

	log(ctx, level, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelDebug, msg, args...)
}

func HttpRequest(ctx context.Context, r *http.Request, status int, duration time.Duration, bytesWritten int64, extraArgs ...any) {
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	} else if status >= 400 {
		level = slog.LevelWarn
	}

	ctxArgs := extractContextValues(ctx)

	stdFields := []any{
		"remote", r.RemoteAddr,
		"method", r.Method,
		"status", status,
		"path", sanitizePath(r.URL.Path),
		"written", humanizeBytes(bytesWritten),
		"duration", humanizeDuration(duration),
	}

	args := make([]any, 0, len(ctxArgs)+len(stdFields)+len(extraArgs))

	if len(ctxArgs) > 0 {
		args = append(args, ctxArgs...)
	}
	args = append(args, stdFields...)
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	}

	logger.Log(ctx, level, "http", args...)
}

func SanitizeURL(urlString string) string {
	parsedURL, err := url.Parse(urlString)

	if err == nil && parsedURL.Host != "" {
		if parsedURL.Path != "" {
			parsedURL.Path = sanitizePath(parsedURL.Path)
			parsedURL.RawPath = ""
		}
		return strings.ReplaceAll(parsedURL.String(), "%2A", "*")
	}

	return sanitizePath(urlString)
}

func sanitizePath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) <= 1 {
		return "/" + strings.Join(parts, "/")
	}

	if len(parts) > 2 {
		parts = []string{"**", parts[len(parts)-1]}
	} else {
		for i := 0; i < len(parts)-1; i++ {
			parts[i] = "*"
		}
	}

	return "/" + strings.Join(parts, "/")
}

func log(ctx context.Context, level slog.Level, msg string, args ...any) {
	ctxArgs := ctxutil.LogFields(ctx)
	if len(ctxArgs) > 0 {
		combinedArgs := make([]any, 0, len(ctxArgs)+len(args))
		combinedArgs = append(combinedArgs, ctxArgs...)
		combinedArgs = append(combinedArgs, args...)
		args = combinedArgs
	}
	logger.Log(ctx, level, msg, args...)
}

func extractContextValues(ctx context.Context) []any {
	return ctxutil.LogFields(ctx)
}

func humanizeBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * 1024
	)

	switch {
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func humanizeDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1000000)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}

	return fmt.Sprintf("%.1fm", d.Minutes())
}
