package agent

import (
	"os"
	"os/exec"
	"testing"
)

// isolatedTmux points tmux at a private server so tests never touch the user's sessions.
func isolatedTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "portal-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-server").Run()
		os.RemoveAll(dir)
	})
}

func TestSessionTargetsDoNotPrefixMatch(t *testing.T) {
	isolatedTmux(t)
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", "test2").CombinedOutput(); err != nil {
		t.Fatalf("start tmux: %v: %s", err, out)
	}
	if tmuxSessionExists("test") {
		t.Fatal(`"test" must not match existing session "test2"`)
	}

	c := &connection{}
	if err := c.createSession("test", ""); err != nil {
		t.Fatalf("createSession(test) with test2 present: %v", err)
	}
	if got := Sessions(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("Portal sessions = %v, want only test (test2 must stay unmarked)", got)
	}
	if err := c.killSession("tes"); err == nil {
		t.Fatal(`killSession("tes") must not resolve to a longer session name`)
	}
	if err := c.killSession("test"); err != nil {
		t.Fatalf("killSession(test): %v", err)
	}
	if !tmuxSessionExists("test2") {
		t.Fatal("killing test removed test2")
	}
}
