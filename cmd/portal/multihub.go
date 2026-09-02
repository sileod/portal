package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sileod/portal/internal/agent"
)

type portalHub struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type portalHubState struct {
	Hubs []portalHub `json:"hubs"`
}

func portalHubsPath() string {
	return filepath.Join(configDir(), "hubs.json")
}

func loadExtraHubs() ([]portalHub, error) {
	data, err := os.ReadFile(portalHubsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state portalHubState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("read hubs: %w", err)
	}
	return cleanHubs(state.Hubs), nil
}

func saveExtraHubs(hubs []portalHub) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(portalHubState{Hubs: cleanHubs(hubs)}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(portalHubsPath(), append(data, '\n'), 0600)
}

func cleanHubs(hubs []portalHub) []portalHub {
	seen := map[string]bool{}
	clean := make([]portalHub, 0, len(hubs))
	for _, hub := range hubs {
		hub.URL = strings.TrimRight(strings.TrimSpace(hub.URL), "/")
		hub.Token = strings.TrimSpace(hub.Token)
		if hub.URL == "" || hub.Token == "" || seen[hub.URL] {
			continue
		}
		seen[hub.URL] = true
		clean = append(clean, hub)
	}
	return clean
}

func configuredHubs(cfg config) []portalHub {
	hubs := []portalHub{{URL: strings.TrimRight(cfg.URL, "/"), Token: cfg.Token}}
	extra, err := loadExtraHubs()
	if err == nil {
		hubs = append(hubs, extra...)
	}
	return cleanHubs(hubs)
}

func normalizeHubURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid Portal URL")
	}
	if u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("Portal URL must not include a path, query, or fragment")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || !(strings.EqualFold(u.Hostname(), "localhost") || ip != nil && ip.IsLoopback()) {
			return "", errors.New("Portal hub URLs require HTTPS; HTTP is allowed only for localhost")
		}
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/"), nil
}

func rememberExtraHub(hub portalHub) error {
	hubs, err := loadExtraHubs()
	if err != nil {
		return err
	}
	next := make([]portalHub, 0, len(hubs)+1)
	next = append(next, hub)
	for _, old := range hubs {
		if strings.TrimRight(old.URL, "/") != strings.TrimRight(hub.URL, "/") {
			next = append(next, old)
		}
	}
	return saveExtraHubs(next)
}

func removeExtraHub(rawURL string) error {
	want, err := normalizeHubURL(rawURL)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err == nil && strings.TrimRight(cfg.URL, "/") == want {
		return errors.New("cannot remove the primary hub; relink Portal first")
	}
	hubs, err := loadExtraHubs()
	if err != nil {
		return err
	}
	found := false
	next := hubs[:0]
	for _, hub := range hubs {
		if strings.TrimRight(hub.URL, "/") == want {
			found = true
			continue
		}
		next = append(next, hub)
	}
	if !found {
		return errors.New("hub is not configured")
	}
	return saveExtraHubs(next)
}

func addHub(args []string) error {
	password := os.Getenv("PORTAL_PASSWORD")
	token := ""
	rawURL := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--password":
			i++
			if i >= len(args) {
				return errors.New("--password requires a value")
			}
			password = args[i]
		case "--token":
			i++
			if i >= len(args) {
				return errors.New("--token requires a value")
			}
			token = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %s", args[i])
			}
			if rawURL != "" {
				return errors.New("usage: portal hub-add URL --password PASSWORD")
			}
			rawURL = args[i]
		}
	}
	if rawURL == "" {
		return errors.New("usage: portal hub-add URL --password PASSWORD")
	}
	hubURL, err := normalizeHubURL(rawURL)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if strings.TrimRight(cfg.URL, "/") == hubURL {
		return errors.New("hub is already the primary Portal URL")
	}
	if token == "" {
		if password == "" {
			return errors.New("Portal password is required to enroll with the additional hub")
		}
		token, err = enrollAgent(hubURL, password)
		if err != nil {
			return err
		}
	}
	if err := rememberExtraHub(portalHub{URL: hubURL, Token: token}); err != nil {
		return err
	}
	if err := restartDaemon(); err != nil {
		return err
	}
	fmt.Printf("✓ added hub %s\n", hubURL)
	return nil
}

func startExtraHubAgents() {
	cfg, err := loadConfig()
	if err != nil {
		return
	}
	primary := strings.TrimRight(cfg.URL, "/")
	for _, hub := range configuredHubs(cfg) {
		if hub.URL == primary {
			continue
		}
		hub := hub
		go func() {
			if err := agent.Run(agent.Config{URL: hub.URL, Token: hub.Token, Host: cfg.Host}); err != nil {
				fmt.Fprintln(os.Stderr, "portal: secondary hub:", err)
			}
		}()
	}
}

type hubProbe struct {
	URL     string
	Latency time.Duration
	OK      bool
}

func fastestHub(hubs []portalHub) string {
	if len(hubs) == 0 {
		return ""
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	results := make(chan hubProbe, len(hubs))
	for _, hub := range hubs {
		hub := hub
		go func() {
			start := time.Now()
			r, err := client.Get(hub.URL + "/login")
			if err != nil {
				results <- hubProbe{URL: hub.URL}
				return
			}
			r.Body.Close()
			results <- hubProbe{URL: hub.URL, Latency: time.Since(start), OK: r.StatusCode < 500}
		}()
	}

	deadline := time.NewTimer(1700 * time.Millisecond)
	defer deadline.Stop()
	var settle *time.Timer
	var settleC <-chan time.Time
	best := hubProbe{}
	remaining := len(hubs)
	for remaining > 0 {
		select {
		case result := <-results:
			remaining--
			if result.OK && (best.URL == "" || result.Latency < best.Latency) {
				best = result
				if settle == nil {
					settle = time.NewTimer(150 * time.Millisecond)
					defer settle.Stop()
					settleC = settle.C
				}
			}
		case <-settleC:
			if best.URL != "" {
				return best.URL
			}
			settleC = nil
		case <-deadline.C:
			remaining = 0
		}
	}
	if best.URL != "" {
		return best.URL
	}
	return hubs[0].URL
}

func openFastestHub() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := ensureDaemon(); err != nil {
		return err
	}
	hubs := configuredHubs(cfg)
	chosen := fastestHub(hubs)
	if chosen == "" {
		return errors.New("no Portal hubs are configured")
	}
	if err := openBrowser(chosen); err != nil {
		fmt.Println(chosen)
	}
	return nil
}

func printHubs() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	hubs := configuredHubs(cfg)
	for i, hub := range hubs {
		kind := "secondary"
		if i == 0 {
			kind = "primary"
		}
		fmt.Printf("%s\t%s\n", kind, hub.URL)
	}
	return nil
}

func preservePrimaryBeforeExpose() {
	if processRunning(hubPIDPath()) {
		return
	}
	cfg, err := loadConfig()
	if err != nil || cfg.URL == "" || cfg.Token == "" {
		return
	}
	_ = rememberExtraHub(portalHub{URL: cfg.URL, Token: cfg.Token})
}

func runMultiHubCommand() (bool, error) {
	if len(os.Args) < 2 {
		return false, nil
	}
	switch os.Args[1] {
	case "daemon":
		startExtraHubAgents()
		return false, nil
	case "hub-add":
		return true, addHub(os.Args[2:])
	case "hub-rm", "hub-remove":
		if len(os.Args) != 3 {
			return true, errors.New("usage: portal hub-rm URL")
		}
		if err := removeExtraHub(os.Args[2]); err != nil {
			return true, err
		}
		if err := restartDaemon(); err != nil {
			return true, err
		}
		fmt.Printf("✓ removed hub %s\n", strings.TrimRight(os.Args[2], "/"))
		return true, nil
	case "hubs":
		return true, printHubs()
	case "open":
		return true, openFastestHub()
	case "expose":
		preservePrimaryBeforeExpose()
		return false, nil
	}
	return false, nil
}

func init() {
	handled, err := runMultiHubCommand()
	if !handled {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "portal:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
