package main

import "testing"

func TestPortalSessionURL(t *testing.T) {
	got := portalSessionURL("https://portal.example/path?keep=1", "gpu-box", "my_session")
	want := "https://portal.example/path?host=gpu-box&keep=1&session=my_session"
	if got != want {
		t.Fatalf("portalSessionURL = %q, want %q", got, want)
	}
}
