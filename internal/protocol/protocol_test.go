package protocol

import "testing"

func TestHostLabel(t *testing.T) {
	for hostname, want := range map[string]string{
		"rack-rack12":         "rack12",
		"rack-rack12.lab.org": "rack12",
		"my-laptop":           "my-laptop",
		"ip-10-0-0-5":         "ip-10-0-0-5",
		"gpu-box":             "gpu-box",
		"a-ab-abc":            "abc",
		"Mac mini":            "Mac-mini",
		"":                    "host",
	} {
		if got := HostLabel(hostname); got != want {
			t.Errorf("HostLabel(%q) = %q, want %q", hostname, got, want)
		}
	}
}
