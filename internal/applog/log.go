package applog

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func InitLogger(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	writer := &ansiUnescapeWriter{w: os.Stdout}

	var h slog.Handler
	h = slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				return slog.String(a.Key, colorizeLevel(a.Value.String()))
			case slog.TimeKey:
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(a.Key, t.Format("2006-01-02 15:04:05.000"))
				}
			case slog.SourceKey:
				if src, ok := a.Value.Any().(*slog.Source); ok {
					return slog.String(a.Key, fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line))
				}
			}
			return a
		},
	})
	h = NewContextValueLogHandler(h)

	logger := slog.New(h)
	slog.SetDefault(logger)
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

func colorizeLevel(level string) string {
	upper := strings.ToUpper(level)
	color := colorGray
	switch upper {
	case "DEBUG":
		color = colorBlue
	case "INFO":
		color = colorGreen
	case "WARN":
		color = colorYellow
	case "ERROR":
		color = colorRed
	}
	return color + upper + colorReset
}

type ansiUnescapeWriter struct {
	w io.Writer
}

func (w *ansiUnescapeWriter) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte(`\x1b`)) && !bytes.Contains(p, []byte(`\u001b`)) {
		return w.w.Write(p)
	}
	b := bytes.ReplaceAll(p, []byte(`\u001b`), []byte{0x1b})
	b = bytes.ReplaceAll(b, []byte(`\x1b`), []byte{0x1b})
	if _, err := w.w.Write(b); err != nil {
		return 0, err
	}
	return len(p), nil
}
