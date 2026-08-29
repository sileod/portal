# Portal

One private URL for terminals on all your machines.

Portal turns persistent `tmux` sessions into browser tabs. Hosts connect outbound to a central hub, so they can live behind NAT or firewalls. The browser talks only to the hub.

```console
$ portal
Portal: https://portal.example.com
Host: workstation

$ portal codex
✓ workstation:codex
https://portal.example.com

$ portal setup
✓ workstation:setup
```

Portal is terminal-first and tool-agnostic. Codex, Claude, Vim, htop, shells, or any other TUI are just terminal processes.

## Architecture

```text
                       https://portal.example.com
                                  │
                           authenticated UI
                                  │
                         ┌────────▼────────┐
                         │   portal hub    │
                         └──────┬───┬──────┘
                                │   │
                    outbound WSS   outbound WSS
                                │   │
                         ┌──────▼┐ ┌▼───────────┐
                         │laptop │ │workstation │
                         │portal │ │portal      │
                         └───┬───┘ └────┬───────┘
                             │          │
                            tmux       tmux
```

The hub authenticates browsers and hosts and routes terminal streams. Hosts never need an inbound port.

## Build

```bash
go build -o portal ./cmd/portal
```

Requirements on terminal hosts: `tmux`.

## Run a hub

Set one high-entropy shared token. Browsers use it to log in and hosts use it to authenticate their outbound connection.

```bash
export PORTAL_TOKEN="$(openssl rand -hex 32)"
export PORTAL_ADDR=:8080
./portal hub
```

Put the hub behind HTTPS (Caddy, Cloudflare Tunnel, nginx, a PaaS, etc.) and expose it at your stable URL.

## Link a host

```bash
portal setup https://portal.example.com --token "$PORTAL_TOKEN"
```

This stores config in `~/.config/portal/config.json` and starts the local daemon. Afterward:

```bash
portal                 # show URL + status
portal codex           # create/keep session "codex"; run codex if it exists
portal paper           # create/keep shell session "paper"
portal gpu -- nvtop    # explicit command
portal ls
portal rm paper
```

The same tmux session remains attachable locally:

```bash
tmux attach -t codex
```

## Web UI

The UI is intentionally only terminals and tabs:

- horizontal or vertical tabs
- light, dark, or system theme
- host-qualified tab names when more than one host is online
- host count in the chrome
- browser reconnect without losing the underlying tmux session
- xterm.js terminal rendering for TUIs, mouse input, resize, paste, and alternate-screen applications

## Security model

The hub requires authentication from the start. The MVP uses a high-entropy token: hosts send it as a bearer token and browsers exchange it for an `HttpOnly`, `Secure`, `SameSite=Strict` cookie. Deploy the hub behind HTTPS.

Terminal traffic is encrypted in transit by HTTPS/WSS but is not yet end-to-end encrypted from browser to host; the hub can see terminal bytes. E2E encryption is a natural next security milestone.

## Development

```bash
PORTAL_TOKEN=dev-token go run ./cmd/portal hub

# another shell
PORTAL_URL=http://localhost:8080 PORTAL_TOKEN=dev-token go run ./cmd/portal daemon
```

Then open `http://localhost:8080`, log in with `dev-token`, and create sessions with `portal NAME`.
