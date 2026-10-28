package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestHelpAndVersionWithoutROS(t *testing.T) {
	for _, flag := range []string{"--help", "--version"} {
		t.Run(flag, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := run([]string{flag}, &out, &errOut); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 || errOut.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
			}
		})
	}
}

func TestSelectedSources(t *testing.T) {
	sources, err := selectedSources([]string{"state", "performance"}, 3*time.Second)
	if err != nil || len(sources) == 0 {
		t.Fatalf("sources=%v err=%v", sources, err)
	}
	for _, source := range sources {
		if source.Group != "state" && source.Group != "performance" {
			t.Fatalf("unexpected group %s", source.Group)
		}
		if !source.Latched && source.MaxAge != 3*time.Second {
			t.Fatalf("periodic source TTL = %v", source.MaxAge)
		}
	}
	if _, err := selectedSources([]string{"typo"}, time.Second); err == nil || !strings.Contains(err.Error(), "typo") {
		t.Fatalf("unknown group accepted: %v", err)
	}
}
