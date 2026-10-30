package agent

import (
	"testing"
	"time"
)

func TestCodexResetFormats(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 13, 0, 0, 0, zone)
	for _, tc := range []struct {
		screen string
		want   time.Time
	}{
		{"You've hit your usage limit. Please try again at 3:42 PM.", time.Date(2026, 10, 30, 15, 42, 0, 0, zone)},
		{"You’ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage\nto purchase more credits or try again at Oct 31st, 2026 12:05 AM.", time.Date(2026, 10, 31, 0, 5, 0, 0, zone)},
		{"usage limit. Try again at Oct 31, 2026, 14:05.", time.Date(2026, 10, 31, 14, 5, 0, 0, zone)},
		{"\x1b[31musage limit\x1b[0m. Try again\nat 15h42.", time.Date(2026, 10, 30, 15, 42, 0, 0, zone)},
		{"usage limit. Try again at 3:42PM.", time.Date(2026, 10, 30, 15, 42, 0, 0, zone)},
		{"usage limit. Try again at 12:05 PM.", time.Date(2026, 10, 30, 12, 5, 0, 0, zone)},
	} {
		got := parseCodexReset(codexResetText(tc.screen), now)
		if got != tc.want.Unix() {
			t.Errorf("%q: %s, want %s", tc.screen, time.Unix(got, 0).In(zone), tc.want)
		}
	}
	for _, screen := range []string{"meeting: try again at 15:42", "5h limit: 0% left (resets 15:42)", "usage limit. Try again later.", "usage limit. Try again at 25:99.", "usage limit. Try again at Feb 30th, 2026 1:00 PM."} {
		if got := parseCodexReset(codexResetText(screen), now); got != 0 {
			t.Errorf("unexpected reset for %q: %d", screen, got)
		}
	}
}

func TestCodexResetExpiresAndDoesNotRollForward(t *testing.T) {
	now := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
	tracker := &screenTracker{seen: map[string]screenState{}}
	screen := "usage limit. Try again at 15:01."
	want := now.Add(time.Minute).Unix()
	if got := tracker.codexReset("codex", screen, now); got != want {
		t.Fatalf("reset=%d, want %d", got, want)
	}
	if got := tracker.codexReset("codex", screen, now.Add(90*time.Second)); got != want {
		t.Fatal("reset expired before its grace period")
	}
	for _, later := range []time.Time{now.Add(2 * time.Minute), now.AddDate(0, 0, 1)} {
		if got := tracker.codexReset("codex", screen, later); got != 0 {
			t.Fatalf("stale reset returned %d", got)
		}
	}
	if got := tracker.codexReset("other", "usage limit. Try again at 14:00.", now); got != 0 {
		t.Fatal("past clock suggested for tomorrow")
	}
	tracker.codexReset("codex", "normal output", now)
	if tracker.seen["codex"].resetAt != 0 {
		t.Fatal("reset persisted after message disappeared")
	}
	tracker.codexReset("codex", screen, now)
	tracker.prune(nil)
	if len(tracker.seen) != 0 {
		t.Fatal("removed session kept reset metadata")
	}
}
