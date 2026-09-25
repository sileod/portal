package agent

import (
	"strings"
	"testing"
	"time"
)

func TestWaitingForInput(t *testing.T) {
	for _, screen := range []string{
		"Bash command\n  rm -rf build\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n",
		"△ Permission required\n  Allow once   Allow always   Reject\n",
		"Overwrite existing file? [y/N] ",
		"Would you like to run the following command?\n  $ make test\n› 1. Yes, proceed (y)\n",
		"Apply this change?\n  ● Yes, allow once\n",
	} {
		if !waitingForInput(screen) {
			t.Errorf("prompt not detected in %q", screen)
		}
	}
	for _, screen := range []string{
		"$ ls\nREADME.md go.mod\n$ ",
		"> \n? for shortcuts            Gemini · high\n",
		"Do you want to proceed?\n" + strings.Repeat("output line\n", promptLines),
	} {
		if waitingForInput(screen) {
			t.Errorf("false prompt detected in %q", screen)
		}
	}
}

func TestScreenTrackerIgnoresIdenticalRedraws(t *testing.T) {
	tr := &screenTracker{seen: map[string]screenState{}}
	start := time.Unix(1000, 0)
	if changed, state := tr.observe("s", "idle prompt", 900, start); changed != 900 || state != "" {
		t.Fatalf("first observation = %d %q, want tmux activity and idle", changed, state)
	}
	if changed, state := tr.observe("s", "idle prompt", 1005, start.Add(5*time.Second)); changed != 900 || state != "" {
		t.Fatalf("identical redraw = %d %q, want unchanged and idle", changed, state)
	}
	if changed, state := tr.observe("s", "spinner ⠧", 1010, start.Add(10*time.Second)); changed != 1010 || state != "working" {
		t.Fatalf("changed screen = %d %q, want 1010 working", changed, state)
	}
	if _, state := tr.observe("s", "spinner ⠧", 1020, start.Add(20*time.Second)); state != "" {
		t.Fatalf("quiet screen state = %q, want idle", state)
	}
	tr.prune(nil)
	if len(tr.seen) != 0 {
		t.Fatal("prune kept a removed session")
	}
}

func TestScreenTrackerIgnoresBlinkingCursor(t *testing.T) {
	tr := &screenTracker{seen: map[string]screenState{}}
	start := time.Unix(1000, 0)
	tr.observe("s", "> █", 900, start)
	// The first flip is indistinguishable from real output.
	if changed, _ := tr.observe("s", "> ", 0, start.Add(time.Second)); changed != 1001 {
		t.Fatalf("first flip changed = %d, want 1001", changed)
	}
	for i := 2; i < 20; i++ {
		frame := "> █"
		if i%2 == 1 {
			frame = "> "
		}
		changed, state := tr.observe("s", frame, 0, start.Add(time.Duration(i)*time.Second))
		if changed != 1001 || (i > 7 && state != "") {
			t.Fatalf("blink %d = %d %q, want 1001 and idle once past the working window", i, changed, state)
		}
	}
	if changed, state := tr.observe("s", "> hi█", 0, start.Add(30*time.Second)); changed != 1030 || state != "working" {
		t.Fatalf("new output = %d %q, want 1030 working", changed, state)
	}
}
