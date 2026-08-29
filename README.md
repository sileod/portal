# Portal

One private URL for terminals on all your machines.

Portal turns persistent `tmux` sessions into browser tabs. Each machine connects outbound to a central authenticated hub, so hosts can sit behind NAT, firewalls, home routers, or institutional networks with no inbound port.

```console
$ portal setup
✓ workstation:setup
https://workstation.example.ts.net

$ portal codex
✓ workstation:codex
https://workstation.example.ts.net
```

Portal is terminal-first and tool-agnostic. Codex, Claude, Vim, htop, shells, SSH, or any other TUI are just terminal processes.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/sileod/portal/main/install.sh | sh
```

The normal install downloads a prebuilt Linux/macOS amd64/arm64 binary. **Go is not a Portal user dependency and the installer never installs Go.** If no binary has been published yet, it may build only when Go is already present on the machine.

The installer puts `portal` in `~/.local/bin`, installs `tmux` if you want it to, then asks how the central URL should be exposed:

```text
Expose the central Portal URL with:
  1) Tailscale Funnel (public HTTPS URL, Portal auth)
  2) Cloudflare Tunnel
  3) Not now
Choice [1]:
Tailscale auth key (leave blank if already connected):
```

The key prompt is hidden. With Tailscale, Portal joins the tailnet if necessary, starts the local hub, configures Tailscale Funnel, discovers the public HTTPS URL, generates the Portal access token, and links this machine to the hub.

After that:

```bash
portal setup
portal codex
portal paper
portal gpu -- nvtop
```

Every Portal-managed terminal appears at the same central URL. With multiple hosts, tabs become `host:session`.

## Explicit setup

The installer is only a convenience. The same provider setup is available directly:

```bash
TAILSCALE_AUTHKEY="$TAILSCALE_AUTHKEY" portal expose tailscale
```

Tailscale Funnel is intentionally used here rather than Tailscale Serve: Funnel exposes the HTTPS endpoint to the public internet, while Portal itself authenticates access to the terminals.

For a remotely managed Cloudflare Tunnel, configure its public hostname to route to `http://127.0.0.1:8080`, then run:

```bash
CLOUDFLARE_TUNNEL_TOKEN="$CLOUDFLARE_TUNNEL_TOKEN" \
portal expose cloudflare --url https://portal.example.com
```

Cloudflare's tunnel token starts the already-configured tunnel; Portal still provides the terminal authentication layer.

To link another machine to an existing Portal instead of hosting the hub there:

```bash
portal link https://your-portal-url --token "$PORTAL_TOKEN"
```

Or simply run `portal` and enter the URL and access token interactively.

## CLI

```text
portal                     show/setup the central Portal URL
portal NAME                create/keep a terminal tab
portal NAME -- COMMAND...  create/keep a tab running COMMAND
portal ls                  list local Portal sessions
portal rm NAME             remove a session
portal open                open the central URL
portal link URL --token T  link this host to an existing hub
portal expose tailscale    host Portal through Tailscale Funnel
portal expose cloudflare   host Portal through Cloudflare Tunnel
portal hub                 run the hub directly
```

If `NAME` resolves to an executable, `portal NAME` runs that executable in the new session. Otherwise it creates a normal shell session. Existing tmux sessions can be opted into Portal by running `portal NAME`; only sessions explicitly marked by Portal are exposed.

The same session remains locally attachable:

```bash
tmux attach -t codex
```

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

The hub authenticates browsers and hosts and routes terminal streams. Terminal hosts never need an inbound port.

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

Authentication is part of Portal from the first version. The current hub uses one high-entropy access token:

- hosts authenticate outbound WebSockets with `Authorization: Bearer …`
- browsers submit the token once and receive an `HttpOnly`, `Secure`, `SameSite=Strict` session cookie
- browser terminal WebSockets require that authenticated cookie and same-origin checks

Tailscale Funnel and Cloudflare Tunnel provide the public HTTPS transport. Portal provides the application authentication.

Terminal traffic is encrypted in transit by HTTPS/WSS but is not yet end-to-end encrypted from browser to host, so the hub can currently see terminal bytes.

## Releases

Every push to `main` can publish/update the rolling `edge` release with prebuilt Linux/macOS amd64/arm64 binaries. Tags matching `v*` publish normal versioned releases. `install.sh` prefers the latest stable release and falls back to `edge`.

## Development

Go 1.22+ is needed only to develop/build Portal from source.

```bash
go build -o portal ./cmd/portal
PORTAL_TOKEN=dev-token ./portal hub
```
