package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sileod/portal/internal/agent"
	"github.com/sileod/portal/internal/auth"
	"github.com/sileod/portal/internal/hub"
	"golang.org/x/term"
)

var errNotLinked = errors.New("not linked")

type config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Host  string `json:"host"`
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
	case "hub":
		password := firstNonEmpty(os.Getenv("PORTAL_PASSWORD"), os.Getenv("PORTAL_TOKEN"))
		if password == "" {
			return errors.New("PORTAL_PASSWORD is required")
		}
		addr := os.Getenv("PORTAL_ADDR")
		if addr == "" {
			addr = ":8080"
		}
		return hub.New(password).Run(addr)
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
	fmt.Printf("✓ %s:%s\n%s\n", cfg.Host, name, cfg.URL)
	return nil
}

func usage() {
	fmt.Print(`portal                        first-run setup, then show the central URL
portal NAME                   create/keep a terminal tab
portal NAME -- COMMAND...     create/keep a tab running COMMAND
portal ls                     list local portal sessions
portal rm NAME                remove a session
portal open                   open the central URL
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
	host, _ := os.Hostname()
	fmt.Printf("Portal host name [%s]: ", host)
	if raw, err := reader.ReadString('\n'); err == nil && strings.TrimSpace(raw) != "" {
		host = strings.TrimSpace(raw)
	}
	secret := strings.TrimSpace(string(password))
	cfg := config{URL: url, Token: auth.AgentToken(secret), Host: host}
	if cfg.URL == "" || secret == "" || cfg.Host == "" {
		return config{}, errors.New("URL, password, and host are required")
	}
	if !validHostAlias(cfg.Host) {
		return config{}, errors.New("host name may contain only letters, digits, _, and -")
	}
	if err := saveConfig(cfg); err != nil {
		return config{}, err
	}
	fmt.Printf("✓ linked %s\n", cfg.Host)
	return cfg, nil
}

func link(args []string) error {
	cfg := config{Token: os.Getenv("PORTAL_TOKEN")}
	if password := os.Getenv("PORTAL_PASSWORD"); password != "" {
		cfg.Token = auth.AgentToken(password)
	}
	if h, err := os.Hostname(); err == nil {
		cfg.Host = h
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--password":
			i++
			if i >= len(args) {
				return errors.New("--password requires a value")
			}
			cfg.Token = auth.AgentToken(args[i])
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
	if cfg.URL == "" || cfg.Token == "" {
		return errors.New("usage: portal link https://portal.example.com --password PASSWORD")
	}
	if cfg.Host == "" {
		return errors.New("could not determine host name; pass --host NAME")
	}
	if !validHostAlias(cfg.Host) {
		return errors.New("host name may contain only letters, digits, _, and -")
	}
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
		if processRunning(hubPIDPath()) {
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
	if exec.Command("tmux", "has-session", "-t", name).Run() != nil {
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

func markPortalSession(name string) error {
	if err := exec.Command("tmux", "set-option", "-t", name, "@portal", "1").Run(); err != nil {
		return err
	}
	if out, err := exec.Command("tmux", "show-option", "-gqv", "@portal_status_bg").Output(); err == nil {
		if color := strings.TrimSpace(string(out)); color != "" {
			exec.Command("tmux", "set-option", "-t", name, "status-style", "bg="+color).Run()
		}
	}
	return nil
}

func isPortalSession(name string) bool {
	out, err := exec.Command("tmux", "show-option", "-qv", "-t", name, "@portal").Output()
	return err == nil && strings.TrimSpace(string(out)) == "1"
}

func killSession(name string) error {
	if !validName(name) {
		return errors.New("invalid session name")
	}
	if !isPortalSession(name) {
		return errors.New("not a Portal-managed session")
	}
	cmd := exec.Command("tmux", "kill-session", "-t", name)
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
func pidPath() string    { return filepath.Join(configDir(), "daemon.pid") }
func logPath() string    { return filepath.Join(configDir(), "daemon.log") }

func loadConfig() (config, error) {
	cfg := config{URL: strings.TrimRight(os.Getenv("PORTAL_URL"), "/"), Token: os.Getenv("PORTAL_TOKEN"), Host: os.Getenv("PORTAL_HOST")}
	if password := os.Getenv("PORTAL_PASSWORD"); password != "" {
		cfg.Token = auth.AgentToken(password)
	}
	data, err := os.ReadFile(configPath())
	if err == nil {
		var fileCfg config
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if cfg.URL == "" {
			cfg.URL = fileCfg.URL
		}
		if cfg.Token == "" {
			cfg.Token = fileCfg.Token
		}
		if cfg.Host == "" {
			cfg.Host = fileCfg.Host
		}
	} else if !os.IsNotExist(err) {
		return cfg, err
	}
	if cfg.Host == "" {
		cfg.Host, _ = os.Hostname()
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
	data, err := json.MarshalIndent(cfg, "", "  ")
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

func daemonRunning() bool {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func restartDaemon() error {
	if data, err := os.ReadFile(pidPath()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			for range 20 {
				if syscall.Kill(pid, 0) != nil {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
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
