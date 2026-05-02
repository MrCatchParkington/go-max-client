package maxclient

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/pion/logging"
)

func TestPionSlogLoggerFactoryRoutesInfoWarnErrorAndSuppressesDebug(t *testing.T) {
	var buf bytes.Buffer
	factory := pionSlogLoggerFactory{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}

	var _ logging.LoggerFactory = factory
	logger := factory.NewLogger("ice-test")
	logger.Debug("hidden debug")
	logger.Infof("allocation %s", "started")
	logger.Warn("allocation slow")
	logger.Errorf("allocation failed: %s", "timeout")

	output := buf.String()
	for _, want := range []string{
		"pion: allocation started",
		"pion: allocation slow",
		"pion: allocation failed: timeout",
		"pion_scope=ice-test",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected log output to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "hidden debug") {
		t.Fatalf("expected debug log to be suppressed, got %q", output)
	}
}
