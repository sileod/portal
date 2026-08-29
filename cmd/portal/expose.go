package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const hubPort = 8080

func expose(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: portal expose tailscale|cloudflare [options]")
	}
	switch args[0] {
	case "tailscale":
		return exposeTailscale(args[1:])
	case "cloudflare":
		return exposeCloudflare(args[1:])
	default:
		return fmt.Errorf("unknown exposure provider %q", args[0])
	}
}

func exposeTailscale(args []string) error {
	key := firstNonEmpty(os.Getenv("TAILSCALE_AUTHKEY"), os.Getenv("TS_AUTHKEY"))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key":
			i++
			if i >= len(args) {
				return errors.New("--key requires a value")
			}
			key = args[i]
		default:
			return fmt.Errorf("unknown option %s", args[i])
		}
	}
	if _, err := exec.LookPath("tailscale"); err != nil {
		return errors.New("tailscale is required")
	}
	if key != "" {
		if err := tailscaleUp(key); err != nil {
			return err
		}
	} else if exec.Command("tailscale", "status").Run() != nil {
		return errors.New("Tailscale is not connected; pass --key TSKEY or set TAILSCALE_AUTHKEY")
	}

	token, err := accessToken()
	if err != nil {
		return err
	}
	if err := ensureHub(token); err != nil {
		return err
	}
	out, err := exec.Command("tailscale", "funnel", "--bg", "--yes", fmt.Sprintf("http://127.0.0.1:%d", hubPort)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale funnel: %s", strings.TrimSpace(string(out)))
	}
	url := firstHTTPS(string(out))
	if url == "" {
		status, _ := exec.Command("tailscale", "funnel", "status").CombinedOutput()
		url = firstHTTPS(string(status))
	}
	if url == "" {
		url, err = tailscaleURL()
		if err != nil {
			return err
		}
	}
	return finishExpose(url, token, "Tailscale Funnel")
}

func exposeCloudflare(args []string) error {
	key := firstNonEmpty(os.Getenv("TUNNEL_TOKEN"), os.Getenv("CLOUDFLARE_TUNNEL_TOKEN"))
	url := strings.TrimRight(os.Getenv("PORTAL_URL"), "/")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key":
			i++
			if i >= len(args) {
				return errors.New("--key requires a value")
			}
			key = args[i]
		case "--url":
			i++
			if i >= len(args) {
				return errors.New("--url requires a value")
			}
			url = strings.TrimRight(args[i], "/")
		default:
			return fmt.Errorf("unknown option %s", args[i])
		}
	}
	if key == "" || url == "" {
		return errors.New("usage: portal expose cloudflare --key TUNNEL_TOKEN --url https://portal.example.com")
	}
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return errors.New("cloudflared is required")
	}
	token, err := accessToken()
	if err != nil {
		return err
	}
	if err := ensureHub(token); err != nil {
		return err
	}
	if err := ensureCloudflared(key); err != nil {
		return err
	}
	return finishExpose(url, token, "Cloudflare Tunnel")
}

func tailscaleUp(key string) error {
	args := []string{"tailscale", "up", "--auth-key=" + key}
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(args[0], args[1:]...)
	} else if _, err := exec.LookPath("sudo"); err == nil {
		cmd = exec.Command("sudo", args...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tailscale up: %w", err)
	}
	return nil
}

func tailscaleURL() (string, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return "", errors.New("could not determine Tailscale Funnel URL")
	}
	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if json.Unmarshal(out, &status) != nil || status.Self.DNSName == "" {
		return "", errors.New("could not determine Tailscale Funnel URL")
	}
	return "https://" + strings.TrimSuffix(status.Self.DNSName, "."), nil
}

func accessToken() (string, error) {
	cfg, _ := loadConfig()
	if cfg.Token != "" {
		return cfg.Token, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func ensureHub(token string) error {
	if processRunning(hubPIDPath()) {
		return nil
	}
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(hubLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "hub")
	cmd.Env = append(os.Environ(), "PORTAL_TOKEN="+token, fmt.Sprintf("PORTAL_ADDR=127.0.0.1:%d", hubPort))
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()
	if err := os.WriteFile(hubPIDPath(), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600); err != nil {
		return err
	}
	for range 20 {
		if processRunning(hubPIDPath()) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("hub did not start; see " + hubLogPath())
}

func ensureCloudflared(key string) error {
	if processRunning(cloudflarePIDPath()) {
		return nil
	}
	logFile, err := os.OpenFile(cloudflareLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command("cloudflared", "tunnel", "run")
	cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+key)
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()
	return os.WriteFile(cloudflarePIDPath(), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600)
}

func finishExpose(url, token, provider string) error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	cfg := config{URL: strings.TrimRight(url, "/"), Token: token, Host: host}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if err := ensureDaemon(); err != nil {
		return err
	}
	fmt.Printf("✓ %s\nPortal: %s\nAccess token: %s\n", provider, cfg.URL, token)
	return nil
}

func processRunning(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && pid > 0 && syscall.Kill(pid, 0) == nil
}

func firstHTTPS(s string) string {
	for _, field := range strings.Fields(s) {
		field = strings.Trim(field, "|()[]{}<>,;\"'")
		if strings.HasPrefix(field, "https://") {
			return strings.TrimRight(field, "/")
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func hubPIDPath() string        { return configDir() + "/hub.pid" }
func hubLogPath() string        { return configDir() + "/hub.log" }
func cloudflarePIDPath() string { return configDir() + "/cloudflared.pid" }
func cloudflareLogPath() string { return configDir() + "/cloudflared.log" }
