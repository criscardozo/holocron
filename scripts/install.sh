#!/usr/bin/env bash
#
# Holocron installer / updater for the Raspberry Pi (headless, over the terminal).
#
# Downloads the latest published arm64 binary, creates the service user, installs
# the systemd unit and starts the service. Re-running it updates in place. The Pi
# never compiles anything — it only ever receives the prebuilt binary.
#
#   Install / update:   curl -fsSL <raw-url>/install.sh | sudo bash
#   Uninstall:          curl -fsSL <raw-url>/install.sh | sudo bash -s -- --uninstall
#
# Options (environment variables):
#   HOLOCRON_VERSION       release tag to install (default: latest)
#   HOLOCRON_ADDR          listen address (default: :8080)
#   HOLOCRON_MEDIA_PATHS   space-separated dirs to grant the service RW access to
#                          (systemd ReadWritePaths; needed to write .nfo/subtitles)
#   HOLOCRON_BINARY_URL    override the download URL (skips release resolution)
#   HOLOCRON_LOCAL_BINARY  path to an already-present binary (skips the download)
#
set -euo pipefail

REPO="criscardozo/holocron"
BINARY_NAME="holocron"
INSTALL_PATH="/usr/local/bin/holocron"
SERVICE_NAME="holocron"
SERVICE_PATH="/etc/systemd/system/holocron.service"
SERVICE_USER="holocron"
STATE_DIR="/var/lib/holocron"
ASSET="holocron-linux-arm64"

VERSION="${HOLOCRON_VERSION:-latest}"
ADDR="${HOLOCRON_ADDR:-:8080}"
MEDIA_PATHS="${HOLOCRON_MEDIA_PATHS:-}"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

require_root() {
	[ "$(id -u)" -eq 0 ] || die "run as root (use sudo)."
}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

check_arch() {
	local arch
	arch="$(uname -m)"
	case "$arch" in
		aarch64 | arm64) : ;;
		*) die "unsupported architecture '$arch'. Holocron ships an arm64 (64-bit) build; a 64-bit Raspberry Pi OS is required." ;;
	esac
}

download_url() {
	if [ -n "${HOLOCRON_BINARY_URL:-}" ]; then
		printf '%s' "$HOLOCRON_BINARY_URL"
	elif [ "$VERSION" = "latest" ]; then
		printf 'https://github.com/%s/releases/latest/download/%s' "$REPO" "$ASSET"
	else
		printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$VERSION" "$ASSET"
	fi
}

# fetch_binary downloads (or copies) the binary into $1 and verifies its checksum
# against the published .sha256 when available.
fetch_binary() {
	local dest="$1"

	if [ -n "${HOLOCRON_LOCAL_BINARY:-}" ]; then
		log "Using local binary: $HOLOCRON_LOCAL_BINARY"
		[ -f "$HOLOCRON_LOCAL_BINARY" ] || die "local binary not found: $HOLOCRON_LOCAL_BINARY"
		cp "$HOLOCRON_LOCAL_BINARY" "$dest"
		return
	fi

	local url
	url="$(download_url)"
	log "Downloading $url"
	curl -fSL --retry 3 -o "$dest" "$url" \
		|| die "download failed. Is there a published release? See 'make release' in the README."

	# Best-effort checksum verification: compare only the hash so the asset name
	# on the Pi does not have to match the name inside the .sha256 file.
	local sumfile="${dest}.sha256"
	if curl -fsSL --retry 2 -o "$sumfile" "${url}.sha256" 2>/dev/null; then
		local expected actual
		expected="$(awk '{print $1}' "$sumfile" | head -n1)"
		actual="$(sha256sum "$dest" | awk '{print $1}')"
		if [ -n "$expected" ] && [ "$expected" != "$actual" ]; then
			die "checksum mismatch (expected $expected, got $actual). Aborting."
		fi
		log "Checksum verified"
	else
		warn "no .sha256 published for this asset; skipping checksum verification."
	fi
}

ensure_user() {
	if ! id "$SERVICE_USER" >/dev/null 2>&1; then
		log "Creating service user '$SERVICE_USER'"
		useradd --system --home "$STATE_DIR" --create-home --shell /usr/sbin/nologin "$SERVICE_USER"
	fi
}

write_service() {
	log "Writing systemd unit $SERVICE_PATH"

	local rw_line=""
	if [ -n "$MEDIA_PATHS" ]; then
		rw_line="ReadWritePaths=$MEDIA_PATHS"
	fi

	cat >"$SERVICE_PATH" <<EOF
[Unit]
Description=Holocron HTPC manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_PATH --addr $ADDR
Restart=on-failure
RestartSec=3

User=$SERVICE_USER
Group=$SERVICE_USER

Environment=HOLOCRON_DB=$STATE_DIR/holocron.db
StateDirectory=$SERVICE_NAME

# Hardening. Media/library paths must stay readable/writable by this user; pass
# HOLOCRON_MEDIA_PATHS to the installer to add them here.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
$rw_line

[Install]
WantedBy=multi-user.target
EOF
}

install_binary() {
	log "Installing binary to $INSTALL_PATH"
	install -m 0755 "$1" "$INSTALL_PATH"
}

start_service() {
	log "Enabling and starting the service"
	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
	systemctl restart "$SERVICE_NAME"
}

access_url() {
	local ip
	ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
	[ -n "$ip" ] || ip="<ip-de-la-pi>"
	local port="${ADDR##*:}"
	[ -n "$port" ] || port="8080"
	printf 'http://%s:%s' "$ip" "$port"
}

do_install() {
	require_root
	check_arch
	require_cmd curl
	require_cmd sha256sum
	require_cmd systemctl

	local existed="no"
	[ -x "$INSTALL_PATH" ] && existed="yes"

	local tmp
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT

	fetch_binary "$tmp/$BINARY_NAME"
	ensure_user
	install_binary "$tmp/$BINARY_NAME"
	write_service
	start_service

	echo
	if [ "$existed" = "yes" ]; then
		log "Holocron updated."
	else
		log "Holocron installed."
	fi
	systemctl --no-pager status "$SERVICE_NAME" | head -n 4 || true
	echo
	log "Open $(access_url) from another machine on the LAN."
	if [ -z "$MEDIA_PATHS" ]; then
		echo
		warn "The service is hardened with ProtectSystem=strict: it cannot write .nfo"
		warn "or subtitles outside its state dir until you grant access to the media"
		warn "folders. Re-run with HOLOCRON_MEDIA_PATHS=\"/path/one /path/two\"."
	fi
}

do_uninstall() {
	require_root
	require_cmd systemctl

	log "Stopping and disabling the service"
	systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true

	log "Removing unit and binary"
	rm -f "$SERVICE_PATH" "$INSTALL_PATH"
	systemctl daemon-reload

	if id "$SERVICE_USER" >/dev/null 2>&1; then
		log "Removing service user '$SERVICE_USER'"
		userdel "$SERVICE_USER" >/dev/null 2>&1 || true
	fi

	log "Done. The database in $STATE_DIR was left untouched."
}

main() {
	case "${1:-}" in
		--uninstall) do_uninstall ;;
		"" | --install) do_install ;;
		*) die "unknown argument: $1 (use --install or --uninstall)" ;;
	esac
}

main "$@"
