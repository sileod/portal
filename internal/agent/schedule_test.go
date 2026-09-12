package agent

import (
	"strings"
	"testing"
)

func TestScheduleScriptChecksCancellationBeforeEverySend(t *testing.T) {
	script := scheduleScript("@portal_schedule_deadbeef", "%7", "private fixture", 30, 3, 60)
	guard := `test -n "$(tmux show-options -gqv '@portal_schedule_deadbeef')" || exit 0`
	if !strings.Contains(script, guard) {
		t.Fatalf("schedule script has no cancellation guard: %s", script)
	}
	if strings.Index(script, guard) > strings.Index(script, "tmux send-keys") {
		t.Fatal("cancellation guard must run before terminal input")
	}
}

func TestValidScheduleID(t *testing.T) {
	for _, id := range []string{"1", "deadbeef", strings.Repeat("a", 64)} {
		if !validScheduleID(id) {
			t.Errorf("validScheduleID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "DEADBEEF", "../../x", strings.Repeat("a", 65)} {
		if validScheduleID(id) {
			t.Errorf("validScheduleID(%q) = true", id)
		}
	}
}
