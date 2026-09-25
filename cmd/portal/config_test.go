package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func onMachine(t *testing.T, name string) {
	t.Helper()
	prev := hostname
	hostname = func() (string, error) { return name, nil }
	t.Cleanup(func() { hostname = prev })
}

func TestHostLabelIsPerMachineInSharedConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_DIR", dir)
	t.Setenv("PORTAL_URL", "")
	t.Setenv("PORTAL_TOKEN", "")
	t.Setenv("PORTAL_HOST", "")
	legacy := `{"url":"https://p.example","token":"tok","host":"portal","auth_version":2}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	host := func(machine string) string {
		t.Helper()
		onMachine(t, machine)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Host
	}

	// The legacy label applies everywhere until claimed.
	if got := host("m6"); got != "portal" {
		t.Fatalf("legacy host on m6 = %q", got)
	}
	// Another machine choosing its own label leaves the legacy one alone.
	cfg, _ := loadConfig()
	cfg.Host = "six"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := host("m10"); got != "portal" {
		t.Fatalf("m10 host = %q after m6 renamed, want portal", got)
	}
	// Claiming the legacy label makes it this machine's only.
	cfg, _ = loadConfig()
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for machine, want := range map[string]string{"m10": "portal", "m6": "six", "m7": "m7"} {
		if got := host(machine); got != want {
			t.Fatalf("%s host = %q, want %q", machine, got, want)
		}
	}
}

func TestPidFromAnotherProcessIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_CONFIG_DIR", dir)
	onMachine(t, "m10")
	// Our own pid is alive but is not a `portal daemon`, as when another
	// machine's pid happens to be in use here.
	if err := os.WriteFile(legacyPIDPath(), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/proc/self/cmdline"); err != nil {
		t.Skip("no /proc")
	}
	if daemonRunning() {
		t.Fatal("daemonRunning trusted a pid that is not a portal daemon")
	}
	if got := filepath.Base(pidPath()); got != "daemon-m10.pid" {
		t.Fatalf("pidPath = %s", got)
	}
}
