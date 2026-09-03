# Portal 🌀

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
- the installer recommends generating a strong random Portal password

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
portal open                  open the fastest reachable Portal hub
portal hubs                  list configured hubs
portal hub-add URL           add another active hub
portal hub-rm URL            remove a secondary hub
portal host NAME             change this machine's Portal label
portal host NAME --tailscale also rename the Funnel hostname
```

You can still attach locally at any time:

```bash
tmux attach -t codex
```

## Multiple hubs / high availability

A terminal host can connect to several Portal hubs at the same time. Every configured hub gets its own outbound agent connection and therefore sees the same machines and tmux sessions.

Add a second hub to a machine with:

```bash
portal hub-add https://portal-home.example.com --password 'your Portal password'
```

Run that on each terminal host that should remain reachable through the second hub. `portal hubs` shows the configured endpoints and `portal hub-rm URL` removes a secondary one.

`portal open` probes all configured hubs concurrently and opens the lowest-latency healthy one. If your home hub is reachable locally, it will normally win over a more distant hub; if it is down, another reachable hub is used instead.

To promote an already joined machine into another hub, expose Portal on it with the same Portal password. Portal preserves its previous primary hub as a secondary connection when doing this. Other terminal hosts still need `portal hub-add NEW_HUB_URL` once so they enroll with the new hub.

Browser login sessions are intentionally local to each hub. If you put a generic reverse proxy/load balancer in front of several hubs, use session affinity for browser/WebSocket traffic. Direct hub URLs plus `portal open` avoid that dependency and give local-first routing.

## Web UI

Portal stays intentionally small and terminal-first.

- vertical tabs by default
- create, rename, and kill sessions
- copy terminal text automatically by selecting it; Ctrl/Cmd+C also works, while Ctrl-C still interrupts when there is no selection
- explicit paste button, plus normal Windows/Linux Ctrl+V and macOS Cmd+V handling
- unread activity + last terminal activity time
- sort tabs by name or recent activity
- schedule arbitrary text + Enter for later
- repeat a scheduled send at an interval
- see pending schedules on the session and in the Portal tab
- update Portal on any connected host from the Portal settings page
- light/dark/system theme
- configurable tab width and tmux status-bar color

A scheduled send is harness-agnostic. For example, schedule `proceed` in `5h`, optionally repeated every `10m`.

The web updater runs the standard Portal installer in a temporary Portal-managed tmux session. On success that session removes itself; on failure it stays open so the installer output can be inspected. Existing tmux sessions are not stopped by an update.

## Persistence

`tmux` is the source of truth. Closing the browser, restarting Portal, or updating the Portal binary does **not** kill your sessions.

Scheduled sends are owned by the host's tmux server, so they also survive Portal/browser restarts. They stop if the tmux server itself stops.

## Update

Use **Update** next to a host in the Portal settings page, or run the installer again:

```bash
curl -fsSL https://raw.githubusercontent.com/sileod/portal/main/install.sh | sh
```

Portal replaces/restarts its own processes and leaves tmux sessions running. Older installs are migrated to the newer credential format; an old joined host may ask for the Portal password once to re-enroll.

## Public URL

Tailscale Funnel is the default because it gives the hub a public HTTPS URL that works from any normal browser.

Cloudflare Tunnel is also supported explicitly:

```bash
CLOUDFLARE_TUNNEL_TOKEN='...' \
PORTAL_PASSWORD='your password' \
portal expose cloudflare --url https://portal.example.com
```

## Security

Portal treats the public URL as a remote-shell login surface:

- passwords are verified with Argon2id, not stored or used as deterministic session keys
- failed login/enrollment attempts are throttled per client IP with exponential backoff
- browser logins mint random, server-side expiring sessions
- terminal hosts receive an independent random 256-bit bearer credential after password enrollment over HTTPS
- additional-hub bearer credentials are stored locally in the Portal config directory with mode `0600`
- cookies are HttpOnly + SameSite=Strict and terminal WebSockets require the authenticated browser session

Traffic is encrypted in transit with HTTPS/WSS. Portal does not yet provide end-to-end encryption between browser and terminal host, so the hub can currently see terminal bytes.

## Development

Go 1.22+ is only needed to build Portal from source.

```bash
go build -o portal ./cmd/portal
```
