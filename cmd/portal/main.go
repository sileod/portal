package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sileod/portal/internal/agent"
	"github.com/sileod/portal/internal/buildinfo"
	"github.com/sileod/portal/internal/hub"
	"github.com/sileod/portal/internal/protocol"
	"golang.org/x/term"
)

var errNotLinked = errors.New("not linked")

type config struct {
	URL         string
	Token       string
	Host        string
	AuthVersion int
}

// fileConfig is config.json. Host labels are kept per machine because a home
// directory shared over NFS gives every machine the same file; one shared
// label would make their agents evict each other at the hub. The legacy
// top-level host still applies to machines without their own entry.
type fileConfig struct {
	URL         string            `json:"url"`
	Token       string            `json:"token"`
	Host        string            `json:"host,omitempty"`
	Hosts       map[string]string `json:"hosts,omitempty"`
	AuthVersion int               `json:"auth_version,omitempty"`
}

var hostname = os.Hostname

// defaultHostLabel is the label this machine gets unless one is chosen.
func defaultHostLabel() string { return protocol.HostLabel(machineName()) }

// machineName identifies this machine among those sharing configDir().
func machineName() string {
	if name, err := hostname(); err == nil && name != "" {
		return name
	}
	return "local"
}

func readFileConfig() (fileConfig, error) {
	var fc fileConfig
	data, err := os.ReadFile(configPath())
	if err != nil {
		return fc, err
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, fmt.Errorf("read config: %w", err)
	}
	return fc, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "portal:", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		cfg, err := loadConfig()
		if errors.Is(err, errNotLinked) {
			cfg, err = interactiveSetup()
		}
		if err != nil {
			return err
		}
		if err := ensureDaemon(); err != nil {
			return err
		}
		printPortal(cfg)
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println(buildinfo.Current())
		return nil
	case "hub":
		state, err := loadOrBootstrapHubAuth()
		if err != nil {
			return err
		}
		addr := os.Getenv("PORTAL_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		server := hub.New(state.PasswordHash, state.AgentToken)
		if err := server.PersistSessions(filepath.Join(configDir(), "hub-sessions.json")); err != nil {
			return err
		}
		if err := server.PersistNotify(filepath.Join(configDir(), "hub-notify.json")); err != nil {
			return err
		}
		return server.Run(addr)
	case "auth-init":
		password := firstNonEmpty(os.Getenv("PORTAL_PASSWORD"), os.Getenv("PORTAL_TOKEN"))
		state, err := ensureHubAuth(password)
		if err != nil {
			return err
		}
		if cfg, err := loadConfig(); err == nil {
			cfg.Token = state.AgentToken
			cfg.AuthVersion = 2
			if err := saveConfig(cfg); err != nil {
				return err
			}
		}
		return nil
	case "daemon":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if err := writePID(); err != nil {
			return err
		}
		defer os.Remove(pidPath())
		return agent.Run(agent.Config{URL: cfg.URL, Token: cfg.Token, Host: cfg.Host})
	case "link":
		return link(args[1:])
	case "expose":
		return expose(args[1:])
	case "host":
		return setHost(args[1:])
	case "ls":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		for _, name := range agent.Sessions() {
			fmt.Printf("%s:%s\n", cfg.Host, name)
		}
		return nil
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: portal rm NAME")
		}
		return killSession(args[1])
	case "open":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if err := ensureDaemon(); err != nil {
			return err
		}
		if err := openBrowser(cfg.URL); err != nil {
			fmt.Println(cfg.URL)
			return nil
		}
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	}

	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, errNotLinked) {
			return errors.New("run `portal` once to link this machine")
		}
		return err
	}
	name := args[0]
	var command []string
	if len(args) > 1 {
		if args[1] != "--" {
			return errors.New("use `portal NAME -- COMMAND...` for an explicit command")
		}
		command = args[2:]
		if len(command) == 0 {
			return errors.New("command missing after --")
		}
	} else if _, err := exec.LookPath(name); err == nil {
		command = []string{name}
	}
	if err := createSession(name, command); err != nil {
		return err
	}
	if err := ensureDaemon(); err != nil {
		return err
	}
	fmt.Printf("✓ %s:%s\nWeb: %s\n", cfg.Host, name, portalSessionURL(cfg.URL, cfg.Host, name))
	return attachSession(name)
}

func usage() {
	fmt.Print(`portal                        first-run setup, then show the central URL
portal NAME                   create/reuse a terminal and attach to it
portal NAME -- COMMAND...     create/reuse, run COMMAND, and attach
portal ls                     list local portal sessions
portal rm NAME                remove a session
portal open                   open the central URL
portal version                show the installed Portal version
portal host NAME              override this host label
portal host NAME --tailscale  also rename this Funnel/MagicDNS host
portal link URL --password P  link/relink this host with the Portal password
portal expose tailscale       run the hub through Tailscale Funnel
portal expose cloudflare      run the hub through Cloudflare Tunnel
portal hub                    run the central hub
`)
}

func interactiveSetup() (config, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return config{}, errors.New("not linked; run `portal link URL --password PASSWORD` or run `portal` interactively")
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Portal URL: ")
	url, err := reader.ReadString('\n')
	if err != nil {
		return config{}, err
	}
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	fmt.Print("Portal password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return config{}, err
	}
	host := defaultHostLabel()
	fmt.Printf("Portal host name [%s]: ", host)
	if raw, err := reader.ReadString('\n'); err == nil && strings.TrimSpace(raw) != "" {
		host = strings.TrimSpace(raw)
	}
	secret := strings.TrimSpace(string(password))
	if url == "" || secret == "" || host == "" {
		return config{}, errors.New("URL, password, and host are required")
	}
	if !validHostAlias(host) {
		return config{}, errors.New("host name may contain only letters, digits, _, and -")
	}
	token, err := enrollAgent(url, secret)
	if err != nil {
		return config{}, err
	}
	cfg := config{URL: url, Token: token, Host: host, AuthVersion: 2}
	if err := saveConfig(cfg); err != nil {
		return config{}, err
	}
	fmt.Printf("✓ linked %s\n", cfg.Host)
	return cfg, nil
}

func link(args []string) error {
	cfg := config{Token: os.Getenv("PORTAL_TOKEN")}
	password := os.Getenv("PORTAL_PASSWORD")
	cfg.Host = defaultHostLabel()
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
			cfg.Token = args[i]
		case "--host":
			i++
			if i >= len(args) {
				return errors.New("--host requires a value")
			}
			cfg.Host = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %s", args[i])
			}
			if cfg.URL != "" {
				return errors.New("only one portal URL may be supplied")
			}
			cfg.URL = strings.TrimRight(args[i], "/")
		}
	}
	if cfg.URL == "" {
		cfg.URL = strings.TrimRight(os.Getenv("PORTAL_URL"), "/")
	}
	if cfg.URL == "" {
		return errors.New("usage: portal link https://portal.example.com --password PASSWORD")
	}
	if cfg.Token == "" {
		if password == "" {
			return errors.New("Portal password is required for enrollment")
		}
		token, err := enrollAgent(cfg.URL, password)
		if err != nil {
			return err
		}
		cfg.Token = token
	}
	if cfg.Host == "" {
		return errors.New("could not determine host name; pass --host NAME")
	}
	if !validHostAlias(cfg.Host) {
		return errors.New("host name may contain only letters, digits, _, and -")
	}
	cfg.AuthVersion = 2
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if err := restartDaemon(); err != nil {
		return err
	}
	fmt.Printf("✓ linked %s\n%s\n", cfg.Host, cfg.URL)
	return nil
}

func setHost(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: portal host NAME [--tailscale]")
	}
	name := args[0]
	if !validHostAlias(name) {
		return errors.New("host name may contain only letters, digits, _, and -")
	}
	renameTailscale := false
	if len(args) == 2 {
		if args[1] != "--tailscale" {
			return fmt.Errorf("unknown option %s", args[1])
		}
		renameTailscale = true
		if !validTailscaleHostname(name) {
			return errors.New("Tailscale host names must be 1-63 letters, digits, or hyphens and cannot start or end with a hyphen")
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if renameTailscale {
		if _, err := exec.LookPath("tailscale"); err != nil {
			return errors.New("tailscale is required for --tailscale")
		}
		out, err := privilegedCommand("tailscale", "set", "--hostname="+name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("tailscale set hostname: %s", strings.TrimSpace(string(out)))
		}
		if hubRunning() {
			out, err = privilegedCommand("tailscale", "funnel", "--bg", "--yes", fmt.Sprintf("http://127.0.0.1:%d", hubPort)).CombinedOutput()
			if err != nil {
				return fmt.Errorf("tailscale funnel: %s", strings.TrimSpace(string(out)))
			}
		}
		if url, err := waitTailscaleURL(name); err == nil {
			cfg.URL = url
		} else {
			return err
		}
	}
	cfg.Host = name
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if err := restartDaemon(); err != nil {
		return err
	}
	fmt.Printf("✓ host: %s\nPortal: %s\n", cfg.Host, cfg.URL)
	return nil
}

func createSession(name string, command []string) error {
	if !validName(name) {
		return errors.New("session names may contain only letters, digits, _, and -")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux is required")
	}
	if exec.Command("tmux", "has-session", "-t", agent.SessionTarget(name)).Run() != nil {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		args := []string{"new-session", "-d", "-s", name, "-c", cwd}
		if len(command) > 0 {
			args = append(args, shellCommand(command))
		}
		cmd := exec.Command("tmux", args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return markPortalSession(name)
}

func attachSession(name string) error {
	// Detached use (scripts, launchers, cron) keeps the old create-only behavior.
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	fmt.Fprintln(os.Stderr, "Attaching to tmux. Detach anytime with Ctrl-b, then d.")
	args := []string{"attach-session", "-t", agent.SessionTarget(name)}
	if os.Getenv("TMUX") != "" {
		args = []string{"switch-client", "-t", agent.SessionTarget(name)}
	}
	cmd := exec.Command("tmux", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func portalSessionURL(base, host, session string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("host", host)
	q.Set("session", session)
	u.RawQuery = q.Encode()
	return u.String()
}

func markPortalSession(name string) error {
	if err := exec.Command("tmux", "set-option", "-t", agent.SessionTarget(name), "@portal", "1").Run(); err != nil {
		return err
	}
	if out, err := exec.Command("tmux", "show-option", "-gqv", "@portal_status_bg").Output(); err == nil {
		if color := strings.TrimSpace(string(out)); color != "" {
			exec.Command("tmux", "set-option", "-t", agent.SessionTarget(name), "status-style", "bg="+color).Run()
		}
	}
	return nil
}

func isPortalSession(name string) bool {
	out, err := exec.Command("tmux", "show-option", "-qv", "-t", agent.SessionTarget(name), "@portal").Output()
	return err == nil && strings.TrimSpace(string(out)) == "1"
}

func killSession(name string) error {
	if !validName(name) {
		return errors.New("invalid session name")
	}
	if !isPortalSession(name) {
		return errors.New("not a Portal-managed session")
	}
	cmd := exec.Command("tmux", "kill-session", "-t", agent.SessionTarget(name))
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validHostAlias(name string) bool {
	return len(name) <= 80 && validName(name)
}

func validTailscaleHostname(name string) bool {
	if len(name) == 0 || len(name) > 63 || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func shellCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}

func configDir() string {
	if dir := os.Getenv("PORTAL_CONFIG_DIR"); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "portal")
	}
	return filepath.Join(dir, "portal")
}

func configPath() string { return filepath.Join(configDir(), "config.json") }

// Runtime files are per machine for the same reason as host labels: a pid
// from another machine sharing configDir() means nothing here.
func machineFile(prefix, ext string) string {
	return filepath.Join(configDir(), prefix+"-"+machineName()+ext)
}

func pidPath() string       { return machineFile("daemon", ".pid") }
func logPath() string       { return machineFile("daemon", ".log") }
func legacyPIDPath() string { return filepath.Join(configDir(), "daemon.pid") }

func loadConfig() (config, error) {
	cfg := config{URL: strings.TrimRight(os.Getenv("PORTAL_URL"), "/"), Token: os.Getenv("PORTAL_TOKEN"), Host: os.Getenv("PORTAL_HOST")}
	fc, err := readFileConfig()
	if err == nil {
		if cfg.URL == "" {
			cfg.URL = fc.URL
		}
		if cfg.Token == "" {
			cfg.Token = fc.Token
		}
		if cfg.Host == "" {
			cfg.Host = firstNonEmpty(fc.Hosts[machineName()], fc.Host)
		}
		cfg.AuthVersion = fc.AuthVersion
	} else if !os.IsNotExist(err) {
		return cfg, err
	}
	if cfg.Host == "" {
		cfg.Host = defaultHostLabel()
	}
	if cfg.URL == "" || cfg.Token == "" {
		return cfg, errNotLinked
	}
	return cfg, nil
}

func saveConfig(cfg config) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	fc, err := readFileConfig()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	fc.URL, fc.Token, fc.AuthVersion = cfg.URL, cfg.Token, cfg.AuthVersion
	if fc.Hosts == nil {
		fc.Hosts = map[string]string{}
	}
	fc.Hosts[machineName()] = cfg.Host
	// The legacy label has no known owner; it stays the fallback for other
	// machines until one claims it explicitly.
	if fc.Host == cfg.Host {
		fc.Host = ""
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), append(data, '\n'), 0600)
}

func writePID() error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
}

func daemonPID() int {
	if pid := runningPID("daemon", pidPath(), legacyPIDPath()); pid != 0 {
		return pid
	}
	// The pid file may have been overwritten by another machine sharing
	// configDir() (older versions used one file for all machines).
	return findDaemonProcess()
}

// findDaemonProcess returns a `portal daemon` of this user from /proc, or 0.
func findDaemonProcess() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		if info, err := entry.Info(); err != nil || info.Sys() == nil || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
			continue
		}
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		if len(args) == 2 && filepath.Base(args[0]) == "portal" && args[1] == "daemon" {
			return pid
		}
	}
	return 0
}

func daemonRunning() bool { return daemonPID() != 0 }

func restartDaemon() error {
	for range 2 {
		pid := daemonPID()
		if pid == 0 {
			break
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
		for range 20 {
			if syscall.Kill(pid, 0) != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	_ = os.Remove(pidPath())
	return ensureDaemon()
}

func ensureDaemon() error {
	if daemonRunning() {
		return nil
	}
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "daemon")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()
	for range 20 {
		if daemonRunning() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("daemon did not start; see " + logPath())
}

func printPortal(cfg config) {
	status := "offline"
	if daemonRunning() {
		status = "connected/reconnecting"
	}
	fmt.Printf("Portal: %s\nHost: %s (%s)\n", cfg.URL, cfg.Host, status)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return errors.New("open the URL manually")
	}
	return cmd.Start()
}

func init() {
	log.SetPrefix("portal: ")
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
}
