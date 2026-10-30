#!/bin/bash

set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
INSTALLER="$SCRIPT_DIR/install-macos.sh"
PUBLISHER="$SCRIPT_DIR/publish-release-to-tos.sh"
PASS_COUNT=0
FAIL_COUNT=0
TMP_ROOT=$(/usr/bin/mktemp -d "${TMPDIR:-/tmp}/openmpa-installer-tests.XXXXXX") || exit 1

cleanup() {
  /bin/rm -rf "$TMP_ROOT"
}
trap cleanup EXIT HUP INT TERM

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'ok %d - %s\n' "$PASS_COUNT" "$1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'not ok - %s\n' "$1" >&2
  if [ -s "$TMP_ROOT/output" ]; then
    /usr/bin/sed 's/^/  /' "$TMP_ROOT/output" >&2
  fi
}

expect_success() {
  description=$1
  shift
  : >"$TMP_ROOT/output"
  if "$@" >"$TMP_ROOT/output" 2>&1; then
    pass "$description"
  else
    fail "$description"
  fi
}

expect_failure() {
  description=$1
  shift
  : >"$TMP_ROOT/output"
  if "$@" >"$TMP_ROOT/output" 2>&1; then
    fail "$description"
  else
    pass "$description"
  fi
}

assert_file_contains() {
  description=$1
  pattern=$2
  file=$3
  if /usr/bin/grep -F -- "$pattern" "$file" >/dev/null 2>&1; then
    pass "$description"
  else
    printf 'missing pattern: %s\n' "$pattern" >"$TMP_ROOT/output"
    fail "$description"
  fi
}

expect_success 'installer has valid Bash syntax' /bin/bash -n "$INSTALLER"
expect_success 'publisher has valid Bash syntax' /bin/bash -n "$PUBLISHER"
expect_success 'test file has valid Bash syntax' /bin/bash -n "$0"

assert_file_contains 'installer pins v0.1.5' "VERSION='0.1.5'" "$INSTALLER"
assert_file_contains 'installer pins the public TOS base URL' \
  "BASE_URL='https://openmpa-downloads.tos-cn-beijing.volces.com'" "$INSTALLER"
assert_file_contains 'installer pins the reviewed DMG digest' \
  "DMG_SHA256='ff0ec5399e4bc52f29f416b08ef0c74d6e71af32b4474188ece12bbdc54bafe0'" "$INSTALLER"
assert_file_contains 'installer pins macOS 13.0 minimum' "MIN_MACOS_VERSION='13.0'" "$INSTALLER"

danger_pattern='spctl.*(--global-disable|--master-disable)|csrutil[[:space:]]+disable|codesign.*(--force|--remove-signature|--sign)|xattr.*-cr[[:space:]]+/(Applications|Users)|sudo.*(bash|sh)[[:space:]]'
if /usr/bin/grep -E -- "$danger_pattern" "$INSTALLER" >/dev/null 2>&1; then
  printf 'forbidden command matched installer source\n' >"$TMP_ROOT/output"
  fail 'installer contains no Gatekeeper bypass, resigning, or broad xattr command'
else
  pass 'installer contains no Gatekeeper bypass, resigning, or broad xattr command'
fi

assert_file_contains 'curl rejects non-HTTPS redirects' "--proto-redir '=https'" "$INSTALLER"
assert_file_contains 'filesystem root install is rejected' \
  "--install-dir must not be the filesystem root" "$INSTALLER"
assert_file_contains 'bundle validation rejects nested symbolic links' \
  '-type l -print -quit' "$INSTALLER"
assert_file_contains 'installer uses one target-wide lock inside the install directory' \
  'LOCK_FILE="$INSTALL_DIR/.OpenMPA.install.lock"' "$INSTALLER"
assert_file_contains 'installer uses the macOS advisory lock primitive' \
  '/usr/bin/lockf -s -t 0 9' "$INSTALLER"
assert_file_contains 'installer holds the advisory lock on a file descriptor' \
  'exec 9>"$LOCK_FILE"' "$INSTALLER"
assert_file_contains 'installer tracks the DMG mount explicitly' \
  'MOUNT_ATTACHED=1' "$INSTALLER"
assert_file_contains 'installer recovers a stable interrupted backup' \
  'Recovered the previous OpenMPA application from an interrupted installation.' "$INSTALLER"
assert_file_contains 'privileged staging removes only its quarantine attribute' \
  'sudo /usr/bin/xattr -dr com.apple.quarantine "$STAGE_APP"' "$INSTALLER"
assert_file_contains 'open is included in preflight tools' '/usr/bin/open /usr/bin/osascript' "$INSTALLER"

expect_success 'help is available without preflight' /bin/bash "$INSTALLER" --help
expect_failure 'relative install directory is rejected' \
  /bin/bash "$INSTALLER" --install-dir relative --no-open
expect_failure 'filesystem root install directory is rejected' \
  /bin/bash "$INSTALLER" --install-dir / --no-open
expect_failure 'a path resolving to filesystem root is rejected' \
  /bin/bash "$INSTALLER" --install-dir /Applications/.. --no-open
expect_failure 'unknown installer option is rejected' /bin/bash "$INSTALLER" --not-an-option

MOCK_BIN="$TMP_ROOT/mock-bin"
/bin/mkdir "$MOCK_BIN"
cat >"$MOCK_BIN/id" <<'EOF'
#!/bin/sh
if [ "${1:-}" = '-u' ]; then
  printf '501\n'
else
  exec /usr/bin/id "$@"
fi
EOF
cat >"$MOCK_BIN/uname" <<'EOF'
#!/bin/sh
if [ "${1:-}" = '-s' ]; then
  printf 'Linux\n'
elif [ "${1:-}" = '-m' ]; then
  printf 'x86_64\n'
else
  printf 'Linux\n'
fi
EOF
/bin/chmod +x "$MOCK_BIN/id" "$MOCK_BIN/uname"
expect_failure 'absolute install-dir and no-open parse before non-Darwin rejection' \
  /usr/bin/env PATH="$MOCK_BIN:$PATH" /bin/bash "$INSTALLER" \
    --install-dir "$TMP_ROOT/install target" --no-open
assert_file_contains 'non-Darwin rejection is explicit' 'this installer supports macOS only' "$TMP_ROOT/output"

STAGING_DIR="$TMP_ROOT/staging"
/bin/mkdir "$STAGING_DIR"
DMG_NAME='OpenMPA-v0.1.5-macos-arm64.dmg'
ZIP_NAME='OpenMPA-v0.1.5-macos-arm64.zip'
printf 'fixture dmg\n' >"$STAGING_DIR/$DMG_NAME"
printf 'fixture zip\n' >"$STAGING_DIR/$ZIP_NAME"
DMG_SHA=$(/usr/bin/shasum -a 256 "$STAGING_DIR/$DMG_NAME" | /usr/bin/awk '{print $1}')
ZIP_SHA=$(/usr/bin/shasum -a 256 "$STAGING_DIR/$ZIP_NAME" | /usr/bin/awk '{print $1}')
printf '%s  %s\n%s  %s\n' "$DMG_SHA" "$DMG_NAME" "$ZIP_SHA" "$ZIP_NAME" \
  >"$STAGING_DIR/SHA256SUMS"
cat >"$STAGING_DIR/build-verification.json" <<EOF
{
  "schema": "openmpa-build-verification/v1",
  "version": "0.1.5",
  "tag": "v0.1.5",
  "commit": "f7c7f954a3b25c88fddbd7e64a58544f07481589",
  "platform": "darwin/arm64",
  "verification": {
    "codeSignature": "valid ad-hoc signature",
    "dmg": "hdiutil verify passed",
    "zip": "unzip integrity test passed"
  },
  "artifacts": {
    "dmgSha256": "$DMG_SHA",
    "zipSha256": "$ZIP_SHA"
  },
  "dmg": { "included": true, "verified": true }
}
EOF
FIXTURE_INSTALLER="$TMP_ROOT/install.sh"
/usr/bin/sed "s/^DMG_SHA256=.*/DMG_SHA256='$DMG_SHA'/" "$INSTALLER" >"$FIXTURE_INSTALLER"
FIXTURE_INSTALLER_SHA=$(/usr/bin/shasum -a 256 "$FIXTURE_INSTALLER" | /usr/bin/awk '{print $1}')

expect_success 'publisher check-only validates complete local contracts' \
  /bin/bash "$PUBLISHER" --version 0.1.5 --staging-dir "$STAGING_DIR" \
    --installer "$FIXTURE_INSTALLER" --installer-sha256 "$FIXTURE_INSTALLER_SHA" --check-only

: >"$TMP_ROOT/output"
if /bin/bash "$PUBLISHER" --version 0.1.5 --staging-dir "$STAGING_DIR" \
  --installer "$FIXTURE_INSTALLER" --installer-sha256 "$FIXTURE_INSTALLER_SHA" \
  --dry-run >"$TMP_ROOT/output" 2>&1; then
  pass 'publisher dry-run succeeds without cloud access'
else
  fail 'publisher dry-run succeeds without cloud access'
fi
assert_file_contains 'publisher dry-run marks version assets immutable' \
  "immutable: tos://openmpa-downloads/releases/v0.1.5/$DMG_NAME" "$TMP_ROOT/output"
last_plan_line=$(/usr/bin/awk '/^  (immutable|mutable):/ { line=$0 } END { print line }' "$TMP_ROOT/output")
if [ "$last_plan_line" = '  mutable:   tos://openmpa-downloads/install.sh (last)' ]; then
  pass 'publisher uploads install.sh last'
else
  printf 'last plan line: %s\n' "$last_plan_line" >"$TMP_ROOT/output"
  fail 'publisher uploads install.sh last'
fi
assert_file_contains 'immutable uploads use If-None-Match star' "--if-none-match '*'" "$PUBLISHER"
assert_file_contains 'publisher fixes the TOS data-plane endpoint' \
  "ENDPOINT='tos-cn-beijing.volces.com'" "$PUBLISHER"
assert_file_contains 'publisher resumes by verifying existing immutable objects' \
  'Verified existing immutable object' "$PUBLISHER"

BROKEN_INSTALLER="$TMP_ROOT/broken-install.sh"
/bin/cp "$FIXTURE_INSTALLER" "$BROKEN_INSTALLER"
printf '\nif then\n' >>"$BROKEN_INSTALLER"
BROKEN_INSTALLER_SHA=$(/usr/bin/shasum -a 256 "$BROKEN_INSTALLER" | /usr/bin/awk '{print $1}')
expect_failure 'publisher rejects an installer with invalid Bash syntax' \
  /bin/bash "$PUBLISHER" --version 0.1.5 --staging-dir "$STAGING_DIR" \
    --installer "$BROKEN_INSTALLER" --installer-sha256 "$BROKEN_INSTALLER_SHA" --check-only

UNREVIEWED_INSTALLER="$TMP_ROOT/unreviewed-install.sh"
/bin/cp "$FIXTURE_INSTALLER" "$UNREVIEWED_INSTALLER"
printf '\nprintf "unreviewed code\\n"\n' >>"$UNREVIEWED_INSTALLER"
expect_failure 'publisher rejects custom installer code without a reviewed digest' \
  /bin/bash "$PUBLISHER" --version 0.1.5 --staging-dir "$STAGING_DIR" \
    --installer "$UNREVIEWED_INSTALLER" --check-only

/bin/mv "$STAGING_DIR/$ZIP_NAME" "$STAGING_DIR/$ZIP_NAME.missing"
expect_failure 'publisher rejects a missing required asset' \
  /bin/bash "$PUBLISHER" --version 0.1.5 --staging-dir "$STAGING_DIR" \
    --installer "$FIXTURE_INSTALLER" --installer-sha256 "$FIXTURE_INSTALLER_SHA" --check-only
/bin/mv "$STAGING_DIR/$ZIP_NAME.missing" "$STAGING_DIR/$ZIP_NAME"

RECOVERY_DIR="$TMP_ROOT/recovery-install"
/bin/mkdir "$RECOVERY_DIR"
RECOVERY_DIR=$(CDPATH= cd -- "$RECOVERY_DIR" && pwd -P)
RECOVERY_INSTALLER="$TMP_ROOT/recovery-installer.sh"
/usr/bin/sed "s#^BASE_URL=.*#BASE_URL='https://127.0.0.1:1'#" "$INSTALLER" >"$RECOVERY_INSTALLER"
RECOVERY_LOCK="$RECOVERY_DIR/.OpenMPA.install.lock"
/usr/bin/touch "$RECOVERY_LOCK"
expect_failure 'persistent lock file without a live holder does not block recovery' \
  /bin/bash "$RECOVERY_INSTALLER" --install-dir "$RECOVERY_DIR" --no-open
if /usr/bin/lockf -s -t 0 -k "$RECOVERY_LOCK" /usr/bin/true; then
  pass 'kernel releases the installer lock after a later download failure'
else
  printf 'advisory lock remained held at %s\n' "$RECOVERY_LOCK" >"$TMP_ROOT/output"
  fail 'kernel releases the installer lock after a later download failure'
fi

/usr/bin/lockf -s -t 0 -k "$RECOVERY_LOCK" /bin/sleep 2 &
LOCK_HOLDER_PID=$!
/bin/sleep 0.2
expect_failure 'a live target-wide advisory lock rejects a concurrent installer' \
  /bin/bash "$RECOVERY_INSTALLER" --install-dir "$RECOVERY_DIR" --no-open
assert_file_contains 'concurrent installer rejection is explicit' \
  'another OpenMPA installer is active' "$TMP_ROOT/output"
wait "$LOCK_HOLDER_PID"

if [ "$FAIL_COUNT" -ne 0 ]; then
  printf 'FAILED: %d check(s) failed; %d passed.\n' "$FAIL_COUNT" "$PASS_COUNT" >&2
  exit 1
fi
printf 'PASS: %d installer and publisher checks passed.\n' "$PASS_COUNT"
