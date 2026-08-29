#!/bin/sh
set -eu

REPO="sileod/portal"
INSTALL_DIR="${PORTAL_INSTALL_DIR:-$HOME/.local/bin}"
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
    say "Portal uses Tailscale Funnel for its public HTTPS URL."
    say "Only this host needs Tailscale; browsers opening Portal do not."
    prompt "Install Tailscale using tailscale.com/install.sh? [Y/n] "
    case "${REPLY:-y}" in
        n|N) say "Tailscale is required for the default Funnel setup."; exit 1 ;;
    esac
    curl -fsSL https://tailscale.com/install.sh | sh
}

say "Installing Portal to $BIN"
install_portal
install_tmux
install_tailscale
say ""

key=""
if ! tailscale status >/dev/null 2>&1; then
    say "Tailscale is not connected on this host."
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

case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "Add $INSTALL_DIR to PATH to run: portal" ;;
esac
