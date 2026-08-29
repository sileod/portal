package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sileod/portal/internal/auth"
)

type hubAuthState struct {
	PasswordHash string `json:"password_hash"`
	AgentToken   string `json:"agent_token"`
}

func hubAuthPath() string { return filepath.Join(configDir(), "hub-auth.json") }

func loadHubAuth() (hubAuthState, error) {
	data, err := os.ReadFile(hubAuthPath())
	if err != nil {
		return hubAuthState{}, err
	}
	var state hubAuthState
	if err := json.Unmarshal(data, &state); err != nil {
		return hubAuthState{}, fmt.Errorf("read hub auth: %w", err)
	}
	if !strings.HasPrefix(state.PasswordHash, "$argon2id$") || state.AgentToken == "" {
		return hubAuthState{}, errors.New("invalid hub auth state")
	}
	return state, nil
}

func ensureHubAuth(password string) (hubAuthState, error) {
	if password == "" {
		return hubAuthState{}, errors.New("Portal password is required")
	}
	if state, err := loadHubAuth(); err == nil {
		if !auth.VerifyPassword(state.PasswordHash, password) {
			return hubAuthState{}, errors.New("Portal password does not match the existing hub")
		}
		return state, nil
	} else if !os.IsNotExist(err) {
		return hubAuthState{}, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return hubAuthState{}, err
	}
	token, err := auth.RandomToken()
	if err != nil {
		return hubAuthState{}, err
	}
	state := hubAuthState{PasswordHash: hash, AgentToken: token}
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return hubAuthState{}, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return hubAuthState{}, err
	}
	if err := os.WriteFile(hubAuthPath(), append(data, '\n'), 0600); err != nil {
		return hubAuthState{}, err
	}
	return state, nil
}

func loadOrBootstrapHubAuth() (hubAuthState, error) {
	state, err := loadHubAuth()
	if err == nil {
		return state, nil
	}
	if !os.IsNotExist(err) {
		return hubAuthState{}, err
	}
	password := firstNonEmpty(os.Getenv("PORTAL_PASSWORD"), os.Getenv("PORTAL_TOKEN"))
	if password == "" {
		return hubAuthState{}, errors.New("hub auth is not initialized; run Portal setup or set PORTAL_PASSWORD once")
	}
	return ensureHubAuth(password)
}

func enrollAgent(portalURL, password string) (string, error) {
	portalURL = strings.TrimRight(strings.TrimSpace(portalURL), "/")
	if portalURL == "" || password == "" {
		return "", errors.New("Portal URL and password are required")
	}
	u, err := url.Parse(portalURL)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid Portal URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return "", errors.New("password enrollment requires HTTPS (HTTP is allowed only for localhost)")
	}

	form := url.Values{"password": {password}}
	req, err := http.NewRequest(http.MethodPost, portalURL+"/api/enroll", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("enroll host: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("enroll host: %s", msg)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Token == "" {
		return "", errors.New("enroll host: invalid response")
	}
	return result.Token, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
