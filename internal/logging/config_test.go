package logging

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestOutputFormats(t *testing.T) {
	for _, test := range []struct {
		name    string
		jsonLog bool
	}{
		{name: "console"},
		{name: "json", jsonLog: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			output := make(chan []byte, 1)
			go func() {
				data, _ := io.ReadAll(reader)
				output <- data
			}()

			stderr := os.Stderr
			os.Stderr = writer
			logger, err := NewConfig(test.jsonLog).Build()
			os.Stderr = stderr
			if err != nil {
				t.Fatal(err)
			}
			logger.Info("request completed", zap.String("request_id", "test"), zap.Duration("duration", time.Second))
			logger.Warn("warning")
			logger.Error("failure")
			for range 200 {
				logger.Info("repeated")
			}
			_ = logger.Sync()
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			data := string(<-output)
			lines := strings.Split(strings.TrimSpace(data), "\n")
			if count := strings.Count(data, "repeated"); count != 200 {
				t.Fatalf("got %d repeated logs, want 200 without sampling", count)
			}
			if test.jsonLog {
				if len(lines) != 203 {
					t.Fatalf("got %d JSON log lines, want 203", len(lines))
				}
				if strings.Contains(data, "\x1b") {
					t.Fatal("JSON logs contain ANSI escapes")
				}
				for index, line := range lines {
					var entry map[string]any
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatalf("invalid JSON log: %v", err)
					}
					if index == 0 && (entry["request_id"] != "test" || entry["duration"] != float64(1)) {
						t.Fatalf("structured fields lost: %v", entry)
					}
				}
			} else {
				for _, level := range []string{"\x1b[34mINFO\x1b[0m", "\x1b[33mWARN\x1b[0m", "\x1b[31mERROR\x1b[0m"} {
					if !strings.Contains(data, level) {
						t.Fatalf("missing colored level %q", level)
					}
				}
				if !strings.Contains(lines[0], `"request_id": "test"`) || !strings.Contains(lines[0], `"duration": "1s"`) {
					t.Fatalf("console fields missing: %s", lines[0])
				}
			}
		})
	}
}
