# Portal

One private URL for terminals on all your machines.

Portal turns persistent `tmux` sessions into browser tabs. Each machine connects outbound to a central authenticated hub, so hosts can sit behind NAT, firewalls, home routers, or institutional networks with no inbound port.

```console
$ portal
Portal URL: https://portal.example.com
Access token:
✓ linked workstation
Portal: https://portal.example.com
Host: workstation (connected/reconnecting)

$ portal setup
✓ workstation:setup
https://portal.example.com

$ portal codex
✓ workstation:codex
https://portal.example.com
```

Portal is terminal-first and tool-agnostic. Codex, Claude, Vim, htop, shells, SSH, or any other TUI are just terminal processes.

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

Terminal hosts need `tmux`. The hub does not.

## Run a hub

Set one high-entropy shared token. Browsers use it to log in and hosts use it to authenticate their outbound connection.

```bash
export PORTAL_TOKEN="$(openssl rand -hex 32)"
export PORTAL_ADDR=:8080
./portal hub
```

Put the hub behind HTTPS and expose it at one stable URL. Caddy, Cloudflare Tunnel, nginx, Fly.io, Railway, a VPS, or any equivalent reverse proxy/PaaS is fine.

A container image can run only the hub:

```bash
docker build -t portal .
docker run --rm -p 8080:8080 -e PORTAL_TOKEN="$PORTAL_TOKEN" portal
```

## First run on a machine

Just run:

```bash
portal
```

If the machine has never been linked, Portal asks for the central URL and access token, stores them in `~/.config/portal/config.json`, starts the outbound daemon, and prints the central URL.

For scripts or provisioning:

```bash
portal link https://portal.example.com --token "$PORTAL_TOKEN"
```

Use `--host NAME` to override the machine hostname.

Afterward:

```bash
portal                 # show URL + host status
portal setup           # shell session named setup
portal codex           # session named codex; runs codex if it exists
portal paper           # shell session named paper
portal gpu -- nvtop    # explicit command
portal ls
portal rm paper
portal open
```

If `NAME` resolves to an executable, `portal NAME` runs that executable in the new session. Otherwise it creates a normal shell session. Existing sessions are simply reused.

The same tmux session remains attachable locally:

```bash
tmux attach -t codex
```

## Web UI

The UI is intentionally only terminals and tabs:

- horizontal or vertical tabs, toggled in place
- light, dark, or system theme
- plain session names on one host
- `host:session` tab labels when multiple hosts are online
- online host count in the chrome
- xterm.js terminal rendering for TUIs, mouse input, resize, paste, and alternate-screen applications
- browser disconnect/reconnect without losing the underlying tmux session

No projects, agents, SSH connection manager, IDE, or workflow model is imposed.

## Authentication and transport

Authentication is part of the hub from the first version. The MVP uses one high-entropy access token:

- hosts authenticate outbound WebSockets with `Authorization: Bearer …`
- browsers submit the token once and receive an `HttpOnly`, `Secure`, `SameSite=Strict` session cookie
- browser terminal WebSockets require that authenticated cookie and same-origin checks

Deploy the hub behind HTTPS. Terminal traffic is encrypted in transit by HTTPS/WSS but is not yet end-to-end encrypted from browser to host, so the hub can currently see terminal bytes. E2E encryption is the next meaningful security milestone if Portal becomes a hosted service.

## Development

```bash
PORTAL_TOKEN=dev-token go run ./cmd/portal hub
```

Then, on a machine with tmux:

```bash
go build -o /tmp/portal ./cmd/portal
/tmp/portal link http://localhost:8080 --token dev-token
/tmp/portal setup
/tmp/portal codex
```

Open `http://localhost:8080`, log in with `dev-token`, and the sessions appear as tabs.
