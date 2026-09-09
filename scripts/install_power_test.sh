#!/usr/bin/env bash
# Exercises the power-helper half of install.sh against a temporary unit
# directory and a stub systemctl.
#
# This part of the installer has silently undone a deliberate decision twice:
# once with ReadWritePaths, once by recreating the cloudflared control after it
# had been removed on purpose. Reading it did not catch either. Running it does.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

export SYSTEMD_DIR="$tmp/units"
export STATE_DIR="$tmp/state"
mkdir -p "$SYSTEMD_DIR" "$STATE_DIR" "$tmp/bin"

# Stub systemctl. is-enabled answers from a file the test controls; everything
# else succeeds quietly.
cat >"$tmp/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
if [ "${1:-}" = "is-enabled" ]; then
	grep -qx "$2" "$ENABLED_UNITS" 2>/dev/null && { echo enabled; exit 0; }
	echo disabled; exit 1
fi
exit 0
STUB
chmod +x "$tmp/bin/systemctl"
export PATH="$tmp/bin:$PATH"
export ENABLED_UNITS="$tmp/enabled"

fail() { echo "FAIL: $*" >&2; exit 1; }

# shellcheck disable=SC1090
HOLOCRON_LIB_ONLY=1 . "$here/install.sh"

# ── 1. Everything enabled: every control is installed ────────────────────────
printf 'jellyfin.service\nqbittorrent.service\ncloudflared.service\n' >"$ENABLED_UNITS"
install_power_helpers >/dev/null

for a in restart-jellyfin restart-qbittorrent restart-cloudflared restart-holocron reboot poweroff; do
	[ -f "$SYSTEMD_DIR/holocron-$a.path" ] || fail "$a was not installed"
done

# ── 2. cloudflared disabled: the control goes, and takes its Before= with it ──
# This is the regression. The tunnel was taken down on purpose; an install must
# not hand back a button that starts it again.
printf 'jellyfin.service\nqbittorrent.service\n' >"$ENABLED_UNITS"
install_power_helpers >/dev/null

[ -f "$SYSTEMD_DIR/holocron-restart-cloudflared.path" ] &&
	fail "the cloudflared control came back after a reinstall"
[ -f "$SYSTEMD_DIR/holocron-restart-cloudflared.service" ] &&
	fail "the cloudflared service unit came back after a reinstall"

grep -q 'Before=holocron-restart-cloudflared.path' "$SYSTEMD_DIR/holocron-action-reset.service" &&
	fail "the reset unit still orders itself before a unit that does not exist"

# The ones that should still be there, are.
for a in restart-jellyfin restart-holocron reboot poweroff; do
	[ -f "$SYSTEMD_DIR/holocron-$a.path" ] || fail "$a disappeared"
	grep -q "Before=holocron-$a.path" "$SYSTEMD_DIR/holocron-action-reset.service" ||
		fail "the reset unit no longer orders itself before $a"
done

# ── 3. Nothing installed but Holocron: the machine-level controls remain ─────
# reboot and poweroff have no service behind them and must never be filtered
# out by a missing dependency.
: >"$ENABLED_UNITS"
install_power_helpers >/dev/null
for a in reboot poweroff restart-holocron; do
	[ -f "$SYSTEMD_DIR/holocron-$a.path" ] || fail "$a should not depend on another unit"
done
for a in restart-jellyfin restart-qbittorrent; do
	[ -f "$SYSTEMD_DIR/holocron-$a.path" ] &&
		fail "$a was installed for a service that is not enabled"
done

# ── 4. Re-enabling brings it back, so the rule is a rule and not a one-way door ─
printf 'cloudflared.service\n' >"$ENABLED_UNITS"
install_power_helpers >/dev/null
[ -f "$SYSTEMD_DIR/holocron-restart-cloudflared.path" ] ||
	fail "re-enabling cloudflared did not bring its control back"

# ── 5. The trigger a unit watches is still empty and still per-action ────────
grep -q "PathExists=$STATE_DIR/.reboot-requested" "$SYSTEMD_DIR/holocron-reboot.path" ||
	fail "the reboot path unit does not watch its own trigger"
grep -q 'ExecStart=/usr/bin/systemctl reboot' "$SYSTEMD_DIR/holocron-reboot.service" ||
	fail "the reboot service does not have a fixed ExecStart"

echo "ok: install.sh power helpers"
