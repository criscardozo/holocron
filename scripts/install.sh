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
#   HOLOCRON_ADDR          listen address (default: :8090)
#   HOLOCRON_MEDIA_PATHS   space-separated dirs to grant the service RW access to
#                          (systemd ReadWritePaths; needed to write subtitles)
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

UPDATER_PATH="/usr/local/bin/holocron-update"
UPDATER_UNIT="/etc/systemd/system/holocron-update.path"
UPDATER_SERVICE="/etc/systemd/system/holocron-update.service"
# Where the privileged updater re-fetches this script from. A release asset,
# not raw main: the updater runs as root, so pulling from a branch means running
# whatever is on that branch at the moment the button is pressed. The asset is
# pinned to a tag and comes with a checksum.
INSTALLER_URL="https://github.com/$REPO/releases/latest/download/install.sh"

VERSION="${HOLOCRON_VERSION:-latest}"
# Left empty on purpose: an existing install's settings are reused when these
# are not given, so re-running (or the in-app updater) never silently changes
# the port or drops access to the media folders.
ADDR="${HOLOCRON_ADDR:-}"
MEDIA_PATHS="${HOLOCRON_MEDIA_PATHS:-}"
DEFAULT_ADDR=":8090"

# Scratch directory for the download. It is global on purpose: the EXIT trap
# runs after do_install has returned, so a function-local would already be out
# of scope and `set -u` would abort the cleanup.
WORK_DIR=""
cleanup() {
	if [ -n "${WORK_DIR:-}" ]; then
		rm -rf "$WORK_DIR"
	fi
}
trap cleanup EXIT

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

# carry_over_settings reuses the running configuration for anything the caller
# did not specify, so an update preserves the port and the media paths.
carry_over_settings() {
	[ -f "$SERVICE_PATH" ] || return 0

	if [ -z "$ADDR" ]; then
		ADDR="$(sed -n 's/^ExecStart=.*--addr[ =]\([^ ]*\).*/\1/p' "$SERVICE_PATH" | head -n1)"
		if [ -n "$ADDR" ]; then
			log "Keeping the configured listen address ($ADDR)"
		fi
	fi
	if [ -z "$MEDIA_PATHS" ]; then
		MEDIA_PATHS="$(sed -n 's/^ReadWritePaths=\(.*\)/\1/p' "$SERVICE_PATH" | head -n1)"
		if [ -n "$MEDIA_PATHS" ]; then
			log "Keeping the configured media paths ($MEDIA_PATHS)"
		fi
	fi

	# Explicit: under `set -e`, ending on a false test would abort the whole
	# install — which is exactly what happened when the unit had no
	# ReadWritePaths line, i.e. every default installation.
	return 0
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

# install_updater sets up the privileged helper behind the "Actualizar ahora"
# button. Holocron runs unprivileged and read-only, so it cannot replace its own
# binary; it drops a trigger file that this path unit watches, and the oneshot
# service re-runs this installer as root. Skip it with HOLOCRON_NO_UPDATER=1.
# effective_media_paths reports the ReadWritePaths systemd actually applies,
# which is not the same as what this script just wrote: a drop-in under
# <unit>.d/ can grant them too, and that is the sturdier place for it — a
# permission kept only in the generated template disappears the day someone
# reinstalls without the environment variable, silently.
#
# Asking systemd instead of grepping our own template is the difference between
# reporting the configuration and reporting the effect.
effective_media_paths() {
	systemctl show "$SERVICE_NAME" -p ReadWritePaths --value 2>/dev/null | tr -d '\n'
}

install_updater() {
	if [ -n "${HOLOCRON_NO_UPDATER:-}" ]; then
		log "Skipping the update helper (HOLOCRON_NO_UPDATER is set)"
		rm -f "$UPDATER_UNIT" "$UPDATER_SERVICE" "$UPDATER_PATH"
		return
	fi

	log "Installing the update helper"
	cat >"$UPDATER_PATH" <<EOF
#!/usr/bin/env bash
# Installed by Holocron. Re-runs the installer to fetch the latest release;
# settings are carried over from the existing unit.
set -euo pipefail
script="\$(mktemp)"
sums="\$(mktemp)"
trap 'rm -f "\$script" "\$sums"' EXIT

curl -fsSL --retry 3 "$INSTALLER_URL" -o "\$script"

# Verify before running: this runs as root, so an installer that arrived
# corrupted or altered would run with everything.
if curl -fsSL --retry 2 "$INSTALLER_URL.sha256" -o "\$sums" 2>/dev/null; then
	expected="\$(awk '{print \$1}' "\$sums" | head -n1)"
	actual="\$(sha256sum "\$script" | awk '{print \$1}')"
	if [ "\$expected" != "\$actual" ]; then
		echo "holocron-update: installer checksum mismatch, refusing to run" >&2
		exit 1
	fi
else
	echo "holocron-update: no checksum published for the installer, refusing to run" >&2
	exit 1
fi

bash "\$script"
EOF
	chmod 0755 "$UPDATER_PATH"

	cat >"$UPDATER_SERVICE" <<EOF
[Unit]
Description=Install the latest Holocron release
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
# Clear the trigger first: the path unit re-arms only once the file is gone, so
# doing this up front means a failed run cannot loop.
ExecStartPre=/bin/rm -f $STATE_DIR/.update-requested
ExecStart=$UPDATER_PATH
TimeoutStartSec=600
EOF

	cat >"$UPDATER_UNIT" <<EOF
[Unit]
Description=Watch for a Holocron update request

[Path]
PathExists=$STATE_DIR/.update-requested
Unit=holocron-update.service

[Install]
WantedBy=multi-user.target
EOF
}

start_service() {
	log "Enabling and starting the service"
	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
	if [ -f "$UPDATER_UNIT" ]; then
		systemctl enable --now holocron-update.path >/dev/null 2>&1 || true
	fi
	# When run by the updater this restarts the very service that asked for it,
	# which is fine: systemd owns both, and the updater runs independently.
	systemctl restart "$SERVICE_NAME"
}

access_url() {
	local ip
	ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
	[ -n "$ip" ] || ip="<ip-de-la-pi>"
	local port="${ADDR##*:}"
	[ -n "$port" ] || port="8090"
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

	WORK_DIR="$(mktemp -d)"

	fetch_binary "$WORK_DIR/$BINARY_NAME"
	ensure_user
	install_binary "$WORK_DIR/$BINARY_NAME"
	carry_over_settings
	[ -n "$ADDR" ] || ADDR="$DEFAULT_ADDR"
	write_service
	install_updater
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
	if [ -z "$(effective_media_paths)" ]; then
		echo
		warn "The service is hardened with ProtectSystem=strict: it cannot write"
		warn "subtitles outside its state dir until you grant access to the media"
		warn "folders. Re-run with HOLOCRON_MEDIA_PATHS=\"/path/one /path/two\","
		warn "or add a drop-in under $SERVICE_NAME.d/ (which survives reinstalls)."
	fi
}

do_uninstall() {
	require_root
	require_cmd systemctl

	log "Stopping and disabling the service"
	systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true

	log "Removing units and binaries"
	systemctl disable --now holocron-update.path >/dev/null 2>&1 || true
	rm -f "$SERVICE_PATH" "$INSTALL_PATH" "$UPDATER_UNIT" "$UPDATER_SERVICE" "$UPDATER_PATH"
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
