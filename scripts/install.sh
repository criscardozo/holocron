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
# Where the units go. A variable rather than a literal so the power-helper
# logic can be run against a temporary directory in a test — this is the part
# of the installer that has silently undone a deliberate decision twice, and
# the only way to know it does not is to exercise it.
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
SERVICE_PATH="$SYSTEMD_DIR/holocron.service"
SERVICE_USER="holocron"
STATE_DIR="/var/lib/holocron"
ASSET="holocron-linux-arm64"

UPDATER_PATH="/usr/local/bin/holocron-update"
UPDATER_UNIT="$SYSTEMD_DIR/holocron-update.path"
UPDATER_SERVICE="$SYSTEMD_DIR/holocron-update.service"
POWER_RESET_SERVICE="$SYSTEMD_DIR/holocron-action-reset.service"
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

# resolve_latest_tag turns the "latest" alias into the tag it currently points
# at, by following the redirect that /releases/latest performs. No jq, no API
# token, no rate limit.
#
# It matters because "latest" is not atomic. While a release is being published,
# the alias can serve one asset from the old release and another from the new
# one, seconds apart — observed in the wild: an install.sh from one version
# alongside the .sha256 of the next. Pinning both downloads to one resolved tag
# makes them consistent by construction instead of by luck.
resolve_latest_tag() {
	local url
	url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest" 2>/dev/null)" || return 1
	case "$url" in
		*/releases/tag/*) printf '%s' "${url##*/}" ;;
		*) return 1 ;;
	esac
}

download_url() {
	if [ -n "${HOLOCRON_BINARY_URL:-}" ]; then
		printf '%s' "$HOLOCRON_BINARY_URL"
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

	if [ "$VERSION" = "latest" ] && [ -z "${HOLOCRON_BINARY_URL:-}" ]; then
		local resolved
		if resolved="$(resolve_latest_tag)"; then
			VERSION="$resolved"
			log "Latest release is $VERSION"
		else
			# Falling back keeps an install working when the redirect cannot be
			# followed; the checksum still has to match.
			warn "could not resolve the latest tag; falling back to the 'latest' alias"
			VERSION="latest-alias"
		fi
	fi

	local url
	url="$(download_url)"
	if [ "$VERSION" = "latest-alias" ]; then
		url="https://github.com/$REPO/releases/latest/download/$ASSET"
	fi
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
			warn "checksum mismatch (expected $expected, got $actual)"
			warn "If a release is being published right now, the binary and its"
			warn "checksum can briefly come from different versions. Wait a"
			warn "minute and try again. If it keeps failing, do not install."
			die "Aborting without installing."
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

# POWER_ACTIONS maps an action name to the command its unit runs. The name is
# the only thing Holocron ever writes, and it writes it as a *filename*, never
# as content: each action gets its own trigger and its own pair of units, so no
# root process ever parses anything the web server produced. The set of things
# that can happen is the set of units installed here, auditable from outside
# Holocron with `systemctl list-units 'holocron-*'`.
#
# KILLS_RESPONDER lists the ones that take down the process serving the request
# that asked for them. Those get a couple of seconds so the HTTP response gets
# out first, otherwise the browser sees a connection reset and cannot tell a
# refusal from a success.
# name <TAB> command <TAB> interrupts-this-request <TAB> unit that must be enabled
#
# The fourth column is what stops Holocron offering to restart something the
# machine does not run. It matters more than convenience: `systemctl restart`
# starts a *disabled* unit — disabled governs boot, not manual start — so a
# button for a service somebody deliberately turned off is a button that turns
# it back on. That happened with cloudflared after the public tunnel was taken
# down, and was measured rather than argued: restart brought the process up and
# it reconnected to the edge.
#
# Empty means always: reboot and poweroff have no service behind them, and
# holocron is the unit this very script just installed.
power_actions() {
	cat <<-'ACTIONS'
	restart-jellyfin	/usr/bin/systemctl restart jellyfin	no	jellyfin.service
	restart-qbittorrent	/usr/bin/systemctl restart qbittorrent	no	qbittorrent.service
	restart-cloudflared	/usr/bin/systemctl restart cloudflared	yes	cloudflared.service
	restart-holocron	/usr/bin/systemctl restart holocron	yes
	reboot	/usr/bin/systemctl reboot	yes
	poweroff	/usr/bin/systemctl poweroff	yes
	ACTIONS
}

# action_wanted reports whether the unit behind an action is enabled on this
# machine. Enablement, not liveness: `is-active` would drop a button because a
# service happened to be restarting, and put it back later, which is worse than
# either answer on its own.
action_wanted() {
	requires="$1"
	[ -n "$requires" ] || return 0
	state="$(systemctl is-enabled "$requires" 2>/dev/null || true)"
	case "$state" in
		enabled | enabled-runtime | static | indirect | alias | generated) return 0 ;;
		*) return 1 ;;
	esac
}

# wanted_power_actions is power_actions filtered to what this machine should
# have. Everything downstream reads from here, including the reset unit's
# Before= lines — otherwise removing an action leaves systemd holding a
# not-found reference and the next person goes looking for a problem that is
# not there.
wanted_power_actions() {
	power_actions | while IFS="$(printf '\t')" read -r name command kills requires; do
		[ -n "$name" ] || continue
		if action_wanted "$requires"; then
			printf '%s\t%s\t%s\n' "$name" "$command" "$kills"
		fi
	done
}

install_power_helpers() {
	if [ -n "${HOLOCRON_NO_POWER:-}" ]; then
		log "Skipping the machine controls (HOLOCRON_NO_POWER is set)"
		remove_power_helpers
		return
	fi

	log "Installing the machine controls"

	# A stale trigger surviving a reboot would be read at boot and acted on
	# again. For a restart that is a nuisance; for poweroff it means the machine
	# shuts itself down every time it starts, recoverable only with a keyboard
	# and a monitor attached. This clears them before any path unit is watching.
	cat >"$POWER_RESET_SERVICE" <<EOF
[Unit]
Description=Clear stale Holocron action triggers
# Ordering that took a reboot to get right. With the default dependencies this
# unit inherits After=basic.target, which runs *after* paths.target; the .path
# units depend on it, so systemd finds a cycle and breaks it by dropping the
# .path units. Everything then looks fine until the next boot, when all of them
# come up "inactive (dead)" and no button works. Verified on the machine: 8 of 8
# active after boot with this ordering, and 0 cycles in the journal.
DefaultDependencies=no
After=local-fs.target
Before=paths.target
# The other half of the DefaultDependencies=no pattern: without it a oneshot
# with RemainAfterExit can sit nominally active through a shutdown. Harmless
# here, but half a pattern invites the next reader to wonder which half is the
# mistake.
Conflicts=shutdown.target
Before=shutdown.target
$(wanted_power_actions | while IFS="$(printf '\t')" read -r name _ _; do
	printf 'Before=holocron-%s.path\n' "$name"
done)

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'rm -f $STATE_DIR/.*-requested'
RemainAfterExit=yes

[Install]
WantedBy=sysinit.target
EOF

	# The full list, not the filtered one, because this loop has to *remove*
	# what is no longer wanted as well as write what is. A run that only ever
	# creates leaves yesterday's decision on disk, which is exactly how the
	# cloudflared button would come back on the next update.
	power_actions | while IFS="$(printf '\t')" read -r name command kills requires; do
		[ -n "$name" ] || continue

		if ! action_wanted "$requires"; then
			if [ -e "$SYSTEMD_DIR/holocron-$name.path" ]; then
				log "  $name: $requires is not enabled, removing the control"
			fi
			systemctl disable --now "holocron-$name.path" >/dev/null 2>&1 || true
			rm -f "$SYSTEMD_DIR/holocron-$name.service" \
				"$SYSTEMD_DIR/holocron-$name.path"
			continue
		fi

		delay=""
		if [ "$kills" = "yes" ]; then
			delay="ExecStartPre=/bin/sleep 2"
		fi

		cat >"$SYSTEMD_DIR/holocron-$name.service" <<EOF
[Unit]
Description=Holocron: $name

[Service]
Type=oneshot
# Clear the trigger first: the path unit re-arms only once the file is gone, so
# a failed run cannot loop.
ExecStartPre=/bin/rm -f $STATE_DIR/.$name-requested
$delay
ExecStart=$command
EOF

		cat >"$SYSTEMD_DIR/holocron-$name.path" <<EOF
[Unit]
Description=Watch for a Holocron $name request
After=holocron-action-reset.service

[Path]
PathExists=$STATE_DIR/.$name-requested
Unit=holocron-$name.service

[Install]
WantedBy=multi-user.target
EOF
	done
}

remove_power_helpers() {
	power_actions | while IFS="$(printf '\t')" read -r name _ _; do
		[ -n "$name" ] || continue
		systemctl disable --now "holocron-$name.path" >/dev/null 2>&1 || true
		rm -f "$SYSTEMD_DIR/holocron-$name.service" \
			"$SYSTEMD_DIR/holocron-$name.path"
	done
	systemctl disable --now holocron-action-reset.service >/dev/null 2>&1 || true
	rm -f "$POWER_RESET_SERVICE"
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

# Resolve which tag "latest" points at, then take both files from that tag.
# The alias is not atomic: during a publication it can serve the installer from
# one release and the checksum from the next, which aborts the update with what
# looks like corruption. Pinning both to one tag makes them consistent by
# construction. Measured on this machine, not hypothetical.
base="https://github.com/$REPO/releases/latest/download"
resolved="\$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
	"https://github.com/$REPO/releases/latest" 2>/dev/null || true)"
case "\$resolved" in
	*/releases/tag/*) base="https://github.com/$REPO/releases/download/\${resolved##*/}" ;;
esac

curl -fsSL --retry 3 "\$base/install.sh" -o "\$script"

# Verify before running: this runs as root, so an installer that arrived
# corrupted or altered would run with everything.
if curl -fsSL --retry 2 "\$base/install.sh.sha256" -o "\$sums" 2>/dev/null; then
	expected="\$(awk '{print \$1}' "\$sums" | head -n1)"
	actual="\$(sha256sum "\$script" | awk '{print \$1}')"
	if [ "\$expected" != "\$actual" ]; then
		echo "holocron-update: installer checksum mismatch." >&2
		echo "holocron-update: if a release is being published right now, this" >&2
		echo "holocron-update: clears up on its own. Try again in a minute." >&2
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
	if [ -f "$POWER_RESET_SERVICE" ]; then
		# Enabled but deliberately not started now: its job is to clear stale
		# triggers at boot, before any path unit watches for them.
		systemctl enable holocron-action-reset.service >/dev/null 2>&1 || true
		wanted_power_actions | while IFS="$(printf '\t')" read -r name _ _; do
			[ -n "$name" ] || continue
			systemctl enable --now "holocron-$name.path" >/dev/null 2>&1 || true
		done
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
	install_power_helpers
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
	remove_power_helpers
	rm -f "$SERVICE_PATH" "$INSTALL_PATH" "$UPDATER_UNIT" "$UPDATER_SERVICE" "$UPDATER_PATH"
	rm -f "$STATE_DIR"/.*-requested
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

# Sourcing the script gives you the functions without running anything, which
# is what the installer test does.
if [ -z "${HOLOCRON_LIB_ONLY:-}" ]; then
	main "$@"
fi
