package main

import "testing"

func TestNormalizeHubURL(t *testing.T) {
	cases := map[string]string{
		"https://EXAMPLE.com/":        "https://example.com",
		"http://localhost:8080/":      "http://localhost:8080",
		"http://127.0.0.1:8080/":      "http://127.0.0.1:8080",
		"https://portal.example.com/": "https://portal.example.com",
	}
	for raw, want := range cases {
		got, err := normalizeHubURL(raw)
		if err != nil || got != want {
			t.Fatalf("normalizeHubURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"http://example.com", "https://example.com/path", "example.com"} {
		if _, err := normalizeHubURL(raw); err == nil {
			t.Fatalf("normalizeHubURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestCleanHubs(t *testing.T) {
	hubs := cleanHubs([]portalHub{
		{URL: "https://a.example/", Token: " a "},
		{URL: "https://a.example", Token: "duplicate"},
		{URL: "https://b.example", Token: "b"},
		{URL: "", Token: "x"},
	})
	if len(hubs) != 2 {
		t.Fatalf("got %d hubs, want 2: %#v", len(hubs), hubs)
	}
	if hubs[0].URL != "https://a.example" || hubs[0].Token != "a" || hubs[1].URL != "https://b.example" {
		t.Fatalf("unexpected hubs: %#v", hubs)
	}
}
