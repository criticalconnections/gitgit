#!/usr/bin/env bash
# Install GitGit and its Cloudflare Tunnel as launchd user agents, so both
# come back after a reboot without anyone logging into a terminal.
#
#   deploy/install-services.sh            install + start
#   deploy/install-services.sh uninstall  stop + remove
#
# These use their own labels (io.gitgit.*) and their own tunnel config, so a
# machine already running other tunnels — including one installed as
# com.cloudflare.cloudflared — is left completely alone.
#
# Note: user agents start at login, not at boot. For a headless server that
# must come up before login, install the same commands as LaunchDaemons in
# /Library/LaunchDaemons (requires sudo).

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="$HOME/Library/LaunchAgents"
LOGS="$HOME/Library/Logs/gitgit"
SERVER_LABEL="io.gitgit.server"
TUNNEL_LABEL="io.gitgit.tunnel"
SERVER_PLIST="$AGENTS/$SERVER_LABEL.plist"
TUNNEL_PLIST="$AGENTS/$TUNNEL_LABEL.plist"

# Configuration — override by exporting before running.
: "${GITGIT_BIN:=$REPO_DIR/gitgit-bin}"
: "${GITGIT_DATA:=$REPO_DIR/data}"
: "${GITGIT_ADDR:=127.0.0.1:3000}"
: "${GITGIT_BASE_URL:=https://gitgit.io}"
: "${GITGIT_PREVIEW_DOMAIN:=preview.gitgit.io}"
: "${GITGIT_OPEN_REGISTRATION:=false}"
: "${TUNNEL_NAME:=gitgit}"
: "${TUNNEL_CONFIG:=$REPO_DIR/deploy/cloudflared.local.yml}"
CLOUDFLARED="$(command -v cloudflared || echo /opt/homebrew/bin/cloudflared)"

unload() {
  for l in "$SERVER_LABEL" "$TUNNEL_LABEL"; do
    launchctl bootout "gui/$(id -u)/$l" 2>/dev/null || launchctl unload "$AGENTS/$l.plist" 2>/dev/null || true
  done
}

if [[ "${1:-}" == "uninstall" ]]; then
  unload
  rm -f "$SERVER_PLIST" "$TUNNEL_PLIST"
  echo "removed $SERVER_LABEL and $TUNNEL_LABEL (other services untouched)"
  exit 0
fi

[[ -x "$GITGIT_BIN" ]] || { echo "no binary at $GITGIT_BIN — run: go build -o gitgit-bin ." >&2; exit 1; }
[[ -f "$TUNNEL_CONFIG" ]] || { echo "no tunnel config at $TUNNEL_CONFIG" >&2; exit 1; }
mkdir -p "$AGENTS" "$LOGS"

# GitGit shells out to git, bash, and whatever a repository's CI or Preview
# Environment needs (node, npm, ...). launchd starts with a bare PATH, so set
# an explicit one.
SERVICE_PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

cat > "$SERVER_PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$SERVER_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$GITGIT_BIN</string>
        <string>-addr</string><string>$GITGIT_ADDR</string>
        <string>-data</string><string>$GITGIT_DATA</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>$SERVICE_PATH</string>
        <key>HOME</key><string>$HOME</string>
        <key>GITGIT_BASE_URL</key><string>$GITGIT_BASE_URL</string>
        <key>GITGIT_PREVIEW_DOMAIN</key><string>$GITGIT_PREVIEW_DOMAIN</string>
        <key>GITGIT_OPEN_REGISTRATION</key><string>$GITGIT_OPEN_REGISTRATION</string>
    </dict>
    <key>WorkingDirectory</key><string>$REPO_DIR</string>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>$LOGS/server.log</string>
    <key>StandardErrorPath</key><string>$LOGS/server.log</string>
</dict>
</plist>
PLIST

cat > "$TUNNEL_PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$TUNNEL_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$CLOUDFLARED</string>
        <string>tunnel</string>
        <string>--config</string><string>$TUNNEL_CONFIG</string>
        <string>run</string><string>$TUNNEL_NAME</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>$SERVICE_PATH</string>
        <key>HOME</key><string>$HOME</string>
    </dict>
    <key>WorkingDirectory</key><string>$REPO_DIR</string>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>$LOGS/tunnel.log</string>
    <key>StandardErrorPath</key><string>$LOGS/tunnel.log</string>
</dict>
</plist>
PLIST

unload
launchctl bootstrap "gui/$(id -u)" "$SERVER_PLIST"
launchctl bootstrap "gui/$(id -u)" "$TUNNEL_PLIST"

echo "installed:"
echo "  $SERVER_LABEL  -> $GITGIT_ADDR (data: $GITGIT_DATA)"
echo "  $TUNNEL_LABEL  -> tunnel '$TUNNEL_NAME' via $TUNNEL_CONFIG"
echo "logs: $LOGS/{server,tunnel}.log"
echo
echo "after rebuilding the binary:  launchctl kickstart -k gui/$(id -u)/$SERVER_LABEL"
