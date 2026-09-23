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
