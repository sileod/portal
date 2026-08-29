#!/bin/sh
set -eu

REPO="sileod/portal"
INSTALL_DIR="${PORTAL_INSTALL_DIR:-$HOME/.local/bin}"
CONFIG_DIR="${PORTAL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/portal}"
TMP="$(mktemp -d)"
trap 'stty echo 2>/dev/null || true; rm -rf "$TMP"' EXIT INT TERM

say() { printf '%s\n' "$*"; }
prompt() {
    printf '%s' "$1" > /dev/tty
    IFS= read -r REPLY < /dev/tty
}
secret() {
    printf '%s' "$1" > /dev/tty
    stty -echo < /dev/tty
    IFS= read -r REPLY < /dev/tty
    stty echo < /dev/tty
    printf '\n' > /dev/tty
}
pid_running() {
    [ -f "$1" ] || return 1
    pid="$(cat "$1" 2>/dev/null || true)"
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}
stop_pidfile() {
    path="$1"
    if pid_running "$path"; then
        pid="$(cat "$path")"
        kill "$pid" 2>/dev/null || true
        i=0
        while kill -0 "$pid" 2>/dev/null && [ "$i" -lt 30 ]; do
            sleep 0.1
            i=$((i+1))
        done
    fi
    rm -f "$path"
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) say "Unsupported architecture: $(uname -m)"; exit 1 ;;
esac
case "$OS" in
    linux|darwin) ;;
    *) say "Unsupported OS: $OS"; exit 1 ;;
esac

mkdir -p "$INSTALL_DIR"
PATH="$INSTALL_DIR:$PATH"
export PATH
BIN="$INSTALL_DIR/portal"
ASSET="portal_${OS}_${ARCH}.tar.gz"
EXISTING=0
[ -f "$CONFIG_DIR/config.json" ] && EXISTING=1
HUB_WAS_RUNNING=0
pid_running "$CONFIG_DIR/hub.pid" && HUB_WAS_RUNNING=1

install_asset() {
    tag="$1"
    [ -n "$tag" ] || return 1
    url="https://github.com/$REPO/releases/download/$tag/$ASSET"
    if curl -fL "$url" -o "$TMP/$ASSET" 2>/dev/null; then
        tar -xzf "$TMP/$ASSET" -C "$TMP"
        install -m 0755 "$TMP/portal" "$BIN"
        return 0
    fi
    return 1
}

build_current_main() {
    src="$TMP/portal-main.tar.gz"
    curl -fL "https://github.com/$REPO/archive/refs/heads/main.tar.gz" -o "$src"
    tar -xzf "$src" -C "$TMP"
    say "No prebuilt Portal binary published yet; building current main with the Go already installed on this machine."
    (
        cd "$TMP/portal-main"
        GOTOOLCHAIN=local go mod tidy
        GOTOOLCHAIN=local go build -o "$TMP/portal" ./cmd/portal
    )
    install -m 0755 "$TMP/portal" "$BIN"
}

install_portal() {
    tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true)"
    if install_asset "$tag" || install_asset edge; then
        return
    fi

    if command -v go >/dev/null 2>&1; then
        build_current_main
        return
    fi

    say "No prebuilt Portal binary is published yet for $OS/$ARCH."
    say "Portal will not install Go. Publish the edge release, then rerun this installer."
    exit 1
}

install_tmux() {
    command -v tmux >/dev/null 2>&1 && return
    prompt "tmux is required for terminal persistence. Install it now? [Y/n] "
    case "${REPLY:-y}" in
        n|N) return ;;
    esac
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update
        sudo apt-get install -y tmux
    elif command -v brew >/dev/null 2>&1; then
        brew install tmux
    else
        say "Install tmux with your package manager before creating Portal tabs."
    fi
}

install_tailscale() {
    command -v tailscale >/dev/null 2>&1 && return
    say "The hub uses Tailscale Funnel for its public HTTPS URL."
    say "Only the hub host needs Tailscale; viewing browsers and joined terminal hosts do not."
    prompt "Install Tailscale using tailscale.com/install.sh? [Y/n] "
    case "${REPLY:-y}" in
        n|N) say "Tailscale is required to create the default Funnel hub."; exit 1 ;;
    esac
    curl -fsSL https://tailscale.com/install.sh | sh
}

choose_new_password() {
    while :; do
        secret "Choose Portal password: "
        password="$REPLY"
        if [ "${#password}" -lt 8 ]; then
            say "Use at least 8 characters. This password protects a remote shell."
            continue
        fi
        secret "Confirm Portal password: "
        [ "$password" = "$REPLY" ] && break
        say "Passwords did not match."
    done
}

restart_existing() {
    say "Existing Portal detected; updating without touching tmux sessions."
    stop_pidfile "$CONFIG_DIR/daemon.pid"

    if [ "$HUB_WAS_RUNNING" -eq 1 ]; then
        hub_pid="$(cat "$CONFIG_DIR/hub.pid" 2>/dev/null || true)"
        password=""
        if [ "$OS" = linux ] && [ -n "$hub_pid" ] && [ -r "/proc/$hub_pid/environ" ]; then
            password="$(tr '\000' '\n' < "/proc/$hub_pid/environ" | sed -n 's/^PORTAL_PASSWORD=//p' | head -n1)"
        fi
        if [ -z "$password" ]; then
            secret "Portal password (needed once to restart the web hub): "
            password="$REPLY"
        fi
        stop_pidfile "$CONFIG_DIR/hub.pid"
        mkdir -p "$CONFIG_DIR"
        PORTAL_PASSWORD="$password" PORTAL_ADDR="127.0.0.1:8080" nohup "$BIN" hub >> "$CONFIG_DIR/hub.log" 2>&1 </dev/null &
        echo $! > "$CONFIG_DIR/hub.pid"
    fi

    "$BIN" >/dev/null
    say "✓ Portal updated"
    say "✓ tmux sessions and scheduled tmux actions were left running"
    exit 0
}

say "Installing Portal to $BIN"
install_portal
install_tmux
if [ "$EXISTING" -eq 1 ]; then
    restart_existing
fi

say ""
say "Portal setup:"
say "  1) Create a new Portal with a public Tailscale Funnel URL"
say "  2) Join an existing Portal as another terminal host"
prompt "Choice [1]: "
mode="${REPLY:-1}"

case "$mode" in
    1)
        install_tailscale
        key=""
        if ! tailscale status >/dev/null 2>&1; then
            say ""
            say "Tailscale is not connected on this hub host."
            say "  1) Sign in in your browser (recommended)"
            say "  2) Use a Tailscale auth key"
            prompt "Choice [1]: "
            case "${REPLY:-1}" in
                1)
                    ;;
                2)
                    say "Create the key in the Tailscale admin console. It is not your Portal password."
                    secret "Tailscale auth key (tskey-...): "
                    key="$REPLY"
                    case "$key" in
                        tskey-*) ;;
                        *) say "That does not look like a Tailscale-generated auth key (expected tskey-...)."; exit 1 ;;
                    esac
                    ;;
                *)
                    say "Unknown choice."
                    exit 1
                    ;;
            esac
        fi

        choose_new_password
        say ""
        say "Creating public Portal URL with Tailscale Funnel..."
        if [ -n "$key" ]; then
            TAILSCALE_AUTHKEY="$key" PORTAL_PASSWORD="$password" "$BIN" expose tailscale
        else
            PORTAL_PASSWORD="$password" "$BIN" expose tailscale
        fi
        say ""
        say "Open the printed URL from any browser and enter your Portal password."
        say "The viewing device does not need Tailscale."
        ;;
    2)
        prompt "Existing Portal URL: "
        url="$REPLY"
        secret "Portal password: "
        password="$REPLY"
        if [ -z "$url" ] || [ -z "$password" ]; then
            say "Portal URL and password are required."
            exit 1
        fi
        PORTAL_PASSWORD="$password" "$BIN" link "$url"
        say ""
        say "Joined. This host connects outbound to the existing Portal; Tailscale is not required here."
        ;;
    *)
        say "Unknown choice."
        exit 1
        ;;
esac

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "Add $INSTALL_DIR to PATH to run: portal" ;;
esac
