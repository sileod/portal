package agent

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sileod/portal/internal/protocol"
)

func TestScheduleScriptChecksCancellationBeforeEverySend(t *testing.T) {
	script := scheduleScript("@portal_schedule_deadbeef", "%7", "private fixture", 30, 3, 60)
	guard := `test -n "$(tmux show-options -gqv '@portal_schedule_deadbeef')" || exit 0`
	if !strings.Contains(script, guard) {
		t.Fatalf("schedule script has no cancellation guard: %s", script)
	}
	if strings.Index(script, guard) > strings.Index(script, "portal_send -t") {
		t.Fatal("cancellation guard must run before terminal input")
	}
	if !strings.Contains(script, "timeout 10 tmux send-keys") || !strings.Contains(script, "portal_send -t") {
		t.Fatal("scheduled terminal sends must have a timeout watchdog")
	}
	skip := `tmux show-options -gqv '@portal_schedule_deadbeef_skip'`
	if !strings.Contains(script, skip) || strings.Index(script, skip) > strings.Index(script, "portal_send -t") {
		t.Fatal("skip-next guard must run before terminal input")
	}
}

func TestSkipNextScheduleIsDurableAndSingleUse(t *testing.T) {
	isolatedTmux(t)
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", "fixture").CombinedOutput(); err != nil {
		t.Fatalf("start tmux: %v: %s", err, out)
	}
	c := &connection{}
	schedule := protocol.Schedule{ID: "deadbeef", Session: "fixture", Pane: "%0", Text: "later", Repeat: 3, Skippable: true}
	if err := c.saveSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	if err := c.skipNextSchedule(schedule.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command("tmux", "show-options", "-gqv", scheduleSkipOption(schedule.ID)).Output()
	if err != nil || strings.TrimSpace(string(raw)) != "1" {
		t.Fatalf("skip option = %q, %v", raw, err)
	}
	if err := c.skipNextSchedule(schedule.ID); err == nil || !strings.Contains(err.Error(), "already skipped") {
		t.Fatalf("second skip error = %v", err)
	}
}

func TestSkippedOccurrenceDoesNotSendButLaterRepeatDoes(t *testing.T) {
	isolatedTmux(t)
	c := &connection{}
	if err := c.createSession("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := c.scheduleInput("fixture", "printf portal-skip-fired", 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	schedules := pendingSchedules()
	if len(schedules) != 1 {
		t.Fatalf("pending schedules = %+v", schedules)
	}
	if err := c.skipNextSchedule(schedules[0].ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1300 * time.Millisecond)
	first, _ := exec.Command("tmux", "capture-pane", "-p", "-t", SessionTarget("fixture")).Output()
	if strings.Contains(string(first), "portal-skip-fired") {
		t.Fatalf("skipped occurrence reached terminal: %q", first)
	}
	time.Sleep(1300 * time.Millisecond)
	second, _ := exec.Command("tmux", "capture-pane", "-p", "-t", SessionTarget("fixture")).Output()
	if !strings.Contains(string(second), "portal-skip-fired") {
		t.Fatalf("later occurrence did not reach terminal: %q", second)
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
