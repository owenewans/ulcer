package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogRemovesLineBreaks(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := (&API{logger: logger}).accessLog(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Method = "GET\r\nforged-method"
	request.URL.Path = "/healthz\r\nforged-path"

	handler.ServeHTTP(httptest.NewRecorder(), request)

	logLine := output.String()
	if strings.Count(logLine, "\n") != 1 {
		t.Fatalf("access log contains injected line break: %q", logLine)
	}
	if strings.Contains(logLine, "\\r") || strings.Contains(logLine, "\\n") {
		t.Fatalf("access log contains escaped line break: %q", logLine)
	}
}
