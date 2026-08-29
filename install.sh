#!/bin/sh
set -eu

REPO="sileod/portal"
INSTALL_DIR="${PORTAL_INSTALL_DIR:-$HOME/.local/bin}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

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

install_go() {
    command -v go >/dev/null 2>&1 && return
    prompt "No Portal release binary exists yet. Install Go to build it now? [Y/n] "
    case "${REPLY:-y}" in
        n|N) say "Cannot install Portal without a release binary or Go."; exit 1 ;;
    esac
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update
        sudo apt-get install -y golang-go
    elif command -v brew >/dev/null 2>&1; then
        brew install go
    else
        say "Install Go and rerun this installer."
        exit 1
    fi
}

install_portal() {
    tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true)"
    if [ -n "$tag" ]; then
        asset="portal_${OS}_${ARCH}.tar.gz"
        url="https://github.com/$REPO/releases/download/$tag/$asset"
        if curl -fL "$url" -o "$TMP/$asset" 2>/dev/null; then
            tar -xzf "$TMP/$asset" -C "$TMP"
            install -m 0755 "$TMP/portal" "$BIN"
            return
        fi
    fi

    install_go
    say "Building Portal..."
    GOBIN="$TMP/bin" go install "github.com/$REPO/cmd/portal@latest"
    install -m 0755 "$TMP/bin/portal" "$BIN"
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
    prompt "Tailscale is not installed. Install it using tailscale.com/install.sh? [Y/n] "
    case "${REPLY:-y}" in
        n|N) say "Tailscale is required for this option."; exit 1 ;;
    esac
    curl -fsSL https://tailscale.com/install.sh | sh
}

install_cloudflared() {
    command -v cloudflared >/dev/null 2>&1 && return
    if [ "$OS" = linux ]; then
        say "Installing cloudflared..."
        curl -fL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-$ARCH" -o "$INSTALL_DIR/cloudflared"
        chmod 0755 "$INSTALL_DIR/cloudflared"
    else
        say "cloudflared is required. Install it first (for example: brew install cloudflared)."
        exit 1
    fi
}

say "Installing Portal to $BIN"
install_portal
install_tmux
say ""
say "Expose the central Portal URL with:"
say "  1) Tailscale Funnel (public HTTPS URL, Portal auth)"
say "  2) Cloudflare Tunnel"
say "  3) Not now"
prompt "Choice [1]: "
choice="${REPLY:-1}"

case "$choice" in
    1)
        install_tailscale
        secret "Tailscale auth key (leave blank if already connected): "
        key="$REPLY"
        if [ -n "$key" ]; then
            TAILSCALE_AUTHKEY="$key" "$BIN" expose tailscale
        else
            "$BIN" expose tailscale
        fi
        ;;
    2)
        install_cloudflared
        say "The Cloudflare tunnel must already route your public hostname to http://127.0.0.1:8080."
        secret "Cloudflare tunnel token: "
        key="$REPLY"
        prompt "Public Portal URL (for example https://portal.example.com): "
        url="$REPLY"
        CLOUDFLARE_TUNNEL_TOKEN="$key" "$BIN" expose cloudflare --url "$url"
        ;;
    3)
        say "Installed. Run: $BIN"
        ;;
    *)
        say "Unknown choice: $choice"
        exit 1
        ;;
esac

case ":${PATH#"$INSTALL_DIR:"}:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "Add $INSTALL_DIR to PATH in future shells to run: portal" ;;
esac
