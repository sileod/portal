# Portal

Your `tmux` sessions, in a browser.

One public URL. One password. Terminals from all your machines as tabs.

Portal is tool-agnostic: Codex, Claude, Vim, htop, shells, SSH, or any other terminal app just works.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/sileod/portal/main/install.sh | sh
```

On the first machine, choose **Create a new Portal**. The default setup uses Tailscale Funnel to give you a public HTTPS URL.

- only the hub machine needs Tailscale
- browsers do not need Tailscale
- Portal login is password-only; there is no username

On another machine, run the same installer and choose **Join an existing Portal**. Enter the Portal URL and the same password. Terminal hosts connect outbound, so they do not need inbound ports.

## Use

```bash
portal codex
portal shell
portal monitor -- htop
portal gpu -- nvtop
```

`portal NAME` creates or reuses a persistent tmux session named `NAME`. If `NAME` is an executable, Portal runs it automatically.

Useful commands:

```text
portal                       show Portal URL/status
portal NAME                  create/reuse a terminal session
portal NAME -- COMMAND...    create/reuse a session running COMMAND
portal ls                    list Portal sessions
portal rm NAME               kill a Portal session
portal open                  open Portal in your browser
portal host NAME             change this machine's Portal label
portal host NAME --tailscale also rename the Funnel hostname
```

You can still attach locally at any time:

```bash
tmux attach -t codex
```

## Web UI

Portal stays intentionally small and terminal-first.

- vertical tabs by default
- create, rename, and kill sessions
- unread activity + last terminal activity time
- sort tabs by name or recent activity
- schedule arbitrary text + Enter for later
- repeat a scheduled send at an interval
- see pending schedules on the session and in the Portal tab
- light/dark/system theme
- configurable tab width and tmux status-bar color

A scheduled send is harness-agnostic. For example, schedule `proceed` in `5h`, optionally repeated every `10m`.

## Persistence

`tmux` is the source of truth. Closing the browser, restarting Portal, or updating the Portal binary does **not** kill your sessions.

Scheduled sends are owned by the host's tmux server, so they also survive Portal/browser restarts. They stop if the tmux server itself stops.

## Update

Run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/sileod/portal/main/install.sh | sh
```

Portal replaces/restarts its own processes and leaves tmux sessions running.

## Public URL

Tailscale Funnel is the default because it gives the hub a public HTTPS URL that works from any normal browser.

Cloudflare Tunnel is also supported explicitly:

```bash
CLOUDFLARE_TUNNEL_TOKEN='...' \
PORTAL_PASSWORD='your password' \
portal expose cloudflare --url https://portal.example.com
```

## Security

Browsers authenticate with the Portal password and receive an HttpOnly session cookie. Terminal hosts use an outbound bearer credential derived from that password.

Traffic is encrypted in transit with HTTPS/WSS. Portal does not yet provide end-to-end encryption between browser and terminal host, so the hub can currently see terminal bytes.

## Development

Go 1.22+ is only needed to build Portal from source.

```bash
go build -o portal ./cmd/portal
```
