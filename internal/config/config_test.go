package config

import (
	"io"
	"testing"
)

func TestJSONLogFlag(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "default"},
		{name: "enabled", args: []string{"--jsonlog"}, want: true},
		{name: "single dash", args: []string{"-jsonlog"}, want: true},
		{name: "disabled", args: []string{"--jsonlog=false"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := Parse(append([]string{"-data", t.TempDir(), "-no-browser"}, test.args...), io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.JSONLog != test.want {
				t.Fatalf("JSONLog = %v, want %v", cfg.JSONLog, test.want)
			}
		})
	}
}
