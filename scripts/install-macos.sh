#!/bin/bash

set -euo pipefail
umask 077

VERSION='0.1.5'
BUNDLE_ID='com.superops.openmpa'
MIN_MACOS_VERSION='13.0'
BASE_URL='https://openmpa-downloads.tos-cn-beijing.volces.com'
DMG_NAME='OpenMPA-v0.1.5-macos-arm64.dmg'
DMG_SHA256='ff0ec5399e4bc52f29f416b08ef0c74d6e71af32b4474188ece12bbdc54bafe0'

INSTALL_DIR='/Applications'
OPEN_AFTER_INSTALL=1
USE_SUDO=0
TMP_DIR=''
MOUNT_POINT=''
MOUNT_ATTACHED=0
TARGET_APP=''
STAGE_APP=''
BACKUP_APP=''
BACKUP_CREATED=0
ACTIVATED=0
INSTALL_COMPLETE=0
CLEANED=0
LOCK_FILE=''
LOCK_HELD=0

say() {
  printf '%s\n' "$*"
}

die() {
  printf 'OpenMPA installer: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install OpenMPA v0.1.5 for macOS Apple Silicon.

Usage: install-macos.sh [--install-dir ABSOLUTE_DIRECTORY] [--no-open]

Options:
  --install-dir DIR  Install OpenMPA.app in an absolute directory.
                     Defaults to /Applications.
  --no-open          Do not open OpenMPA after a successful installation.
  -h, --help         Show this help.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir)
      [ "$#" -ge 2 ] || die '--install-dir requires a value'
      INSTALL_DIR=$2
      shift 2
      ;;
    --no-open)
      OPEN_AFTER_INSTALL=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

case "$INSTALL_DIR" in
  /*) ;;
  *) die '--install-dir must be an absolute directory' ;;
esac
case "$INSTALL_DIR" in
  *"
"*) die '--install-dir must not contain a newline' ;;
esac
while [ "$INSTALL_DIR" != '/' ] && [ "${INSTALL_DIR%/}" != "$INSTALL_DIR" ]; do
  INSTALL_DIR=${INSTALL_DIR%/}
done
[ "$INSTALL_DIR" != '/' ] || die '--install-dir must not be the filesystem root'

version_at_least() {
  host_version=$1
  required_version=$2
  old_ifs=$IFS
  IFS=.
  set -- $host_version
  host_major=${1:-0}
  host_minor=${2:-0}
  set -- $required_version
  required_major=${1:-0}
  required_minor=${2:-0}
  IFS=$old_ifs

  case "$host_major:$host_minor:$required_major:$required_minor" in
    *[!0-9:]*) return 1 ;;
  esac
  [ "$host_major" -gt "$required_major" ] || {
    [ "$host_major" -eq "$required_major" ] &&
      [ "$host_minor" -ge "$required_minor" ]
  }
}

require_executable() {
  [ -x "$1" ] || die "required tool is unavailable: $1"
}

preflight() {
  [ "$(id -u)" -ne 0 ] || die 'do not run this installer as root or with sudo'
  [ "$(uname -s)" = 'Darwin' ] || die 'this installer supports macOS only'
  [ "$(uname -m)" = 'arm64' ] || die 'this installer requires Apple Silicon (arm64)'

  host_version=$(/usr/bin/sw_vers -productVersion)
  version_at_least "$host_version" "$MIN_MACOS_VERSION" ||
    die "macOS ${MIN_MACOS_VERSION} or newer is required (found ${host_version})"

  for tool in \
    /usr/bin/codesign /usr/bin/curl /usr/bin/ditto /usr/bin/find \
    /usr/bin/hdiutil /usr/bin/lipo /usr/bin/lockf /usr/bin/open /usr/bin/osascript \
    /usr/bin/pgrep /usr/bin/shasum /usr/bin/touch /usr/bin/xattr \
    /usr/libexec/PlistBuddy; do
    require_executable "$tool"
  done

  case "$BASE_URL" in
    https://*) ;;
    *) die 'download URL must use HTTPS' ;;
  esac

  if [ ! -e "$INSTALL_DIR" ]; then
    [ "$INSTALL_DIR" != '/Applications' ] || die '/Applications is missing'
    /bin/mkdir -p "$INSTALL_DIR" || die "cannot create install directory: $INSTALL_DIR"
  fi
  [ -d "$INSTALL_DIR" ] || die "install directory is not a directory: $INSTALL_DIR"
  [ ! -L "$INSTALL_DIR" ] || die "install directory must not be a symbolic link: $INSTALL_DIR"
  INSTALL_DIR=$(CDPATH= cd -- "$INSTALL_DIR" && pwd -P)
  [ "$INSTALL_DIR" != '/' ] || die '--install-dir must not resolve to the filesystem root'

  if [ -w "$INSTALL_DIR" ]; then
    USE_SUDO=0
  elif [ "$INSTALL_DIR" = '/Applications' ]; then
    require_executable /usr/bin/sudo
    USE_SUDO=1
  else
    die "custom install directory is not writable: $INSTALL_DIR"
  fi
}

assert_managed_path() {
  case "$1" in
    "$TARGET_APP"|"$STAGE_APP"|"$BACKUP_APP") ;;
    *) die "refusing to modify unmanaged path: $1" ;;
  esac
}

remove_managed_path() {
  assert_managed_path "$1"
  if [ "$USE_SUDO" -eq 1 ]; then
    /usr/bin/sudo /bin/rm -rf "$1"
  else
    /bin/rm -rf "$1"
  fi
}

move_managed_path() {
  assert_managed_path "$1"
  assert_managed_path "$2"
  if [ "$USE_SUDO" -eq 1 ]; then
    /usr/bin/sudo /bin/mv "$1" "$2"
  else
    /bin/mv "$1" "$2"
  fi
}

copy_to_stage() {
  if [ "$USE_SUDO" -eq 1 ]; then
    /usr/bin/sudo /usr/bin/ditto "$1" "$STAGE_APP"
  else
    /usr/bin/ditto "$1" "$STAGE_APP"
  fi
}

cleanup() {
  status=$?
  [ "$CLEANED" -eq 0 ] || exit "$status"
  CLEANED=1
  trap - EXIT HUP INT TERM
  set +e

  if [ "$INSTALL_COMPLETE" -ne 1 ] && [ -n "$TARGET_APP" ]; then
    if [ "$BACKUP_CREATED" -eq 1 ]; then
      if [ "$ACTIVATED" -eq 1 ] && { [ -e "$TARGET_APP" ] || [ -L "$TARGET_APP" ]; }; then
        remove_managed_path "$TARGET_APP"
      fi
      if [ ! -e "$TARGET_APP" ] && [ ! -L "$TARGET_APP" ] &&
        { [ -e "$BACKUP_APP" ] || [ -L "$BACKUP_APP" ]; }; then
        move_managed_path "$BACKUP_APP" "$TARGET_APP"
        printf 'OpenMPA installer: restored the previous application after failure.\n' >&2
      fi
    elif [ "$ACTIVATED" -eq 1 ] && { [ -e "$TARGET_APP" ] || [ -L "$TARGET_APP" ]; }; then
      remove_managed_path "$TARGET_APP"
    fi
    if [ -n "$STAGE_APP" ] && { [ -e "$STAGE_APP" ] || [ -L "$STAGE_APP" ]; }; then
      remove_managed_path "$STAGE_APP"
    fi
  elif [ "$INSTALL_COMPLETE" -eq 1 ] && [ "$BACKUP_CREATED" -eq 1 ] &&
    { [ -e "$BACKUP_APP" ] || [ -L "$BACKUP_APP" ]; }; then
    remove_managed_path "$BACKUP_APP"
  fi

  if [ "$MOUNT_ATTACHED" -eq 1 ]; then
    if /usr/bin/hdiutil detach "$MOUNT_POINT" -quiet >/dev/null 2>&1; then
      MOUNT_ATTACHED=0
    else
      printf 'OpenMPA installer: warning: could not detach temporary DMG mount at %s.\n' \
        "$MOUNT_POINT" >&2
    fi
  fi
  if [ "$MOUNT_ATTACHED" -eq 0 ] && [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    /bin/rm -rf "$TMP_DIR"
  fi
  release_lock
  exit "$status"
}

validate_app() {
  app_path=$1
  label=$2
  [ -d "$app_path" ] || die "$label is not an application directory"
  [ ! -L "$app_path" ] || die "$label must not be a symbolic link"
  if [ -n "$(/usr/bin/find "$app_path" -type l -print -quit)" ]; then
    die "$label must not contain symbolic links"
  fi

  info_plist="$app_path/Contents/Info.plist"
  [ -f "$info_plist" ] && [ ! -L "$info_plist" ] || die "$label has no regular Info.plist"
  /usr/bin/codesign --verify --deep --strict "$app_path" >/dev/null 2>&1 ||
    die "$label failed code-signature verification"

  actual_bundle_id=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$info_plist" 2>/dev/null) ||
    die "$label has no Bundle ID"
  [ "$actual_bundle_id" = "$BUNDLE_ID" ] ||
    die "$label has unexpected Bundle ID: $actual_bundle_id"

  actual_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$info_plist" 2>/dev/null) ||
    die "$label has no bundle version"
  [ "$actual_version" = "$VERSION" ] ||
    die "$label has unexpected version: $actual_version"

  executable_name=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$info_plist" 2>/dev/null) ||
    die "$label has no main executable"
  executable_path="$app_path/Contents/MacOS/$executable_name"
  [ -f "$executable_path" ] && [ ! -L "$executable_path" ] ||
    die "$label has no regular main executable"
  /usr/bin/lipo -archs "$executable_path" 2>/dev/null |
    /usr/bin/tr ' ' '\n' |
    /usr/bin/grep -qx 'arm64' || die "$label main executable does not contain arm64"
}

validate_existing_bundle_id() {
  app_path=$1
  [ -d "$app_path" ] || die 'existing OpenMPA target is not an application directory'
  [ ! -L "$app_path" ] || die 'existing OpenMPA target must not be a symbolic link'
  info_plist="$app_path/Contents/Info.plist"
  [ -f "$info_plist" ] && [ ! -L "$info_plist" ] ||
    die 'existing OpenMPA target has no regular Info.plist'
  actual_bundle_id=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$info_plist" 2>/dev/null) ||
    die 'existing OpenMPA target has no Bundle ID'
  [ "$actual_bundle_id" = "$BUNDLE_ID" ] ||
    die "refusing to replace another application (Bundle ID: $actual_bundle_id)"
}

quiesce_existing_app() {
  /usr/bin/pgrep -x 'OpenMPA' >/dev/null 2>&1 || return 0
  say 'Requesting the running OpenMPA application to quit...'
  /usr/bin/osascript -e "tell application id \"$BUNDLE_ID\" to quit" >/dev/null 2>&1 || true
  attempts=0
  while /usr/bin/pgrep -x 'OpenMPA' >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    [ "$attempts" -lt 20 ] ||
      die 'OpenMPA is still running; quit it normally, then retry the installer'
    /bin/sleep 0.5
  done
}

release_lock() {
  if [ "$LOCK_HELD" -eq 1 ]; then
    exec 9>&-
    LOCK_HELD=0
  fi
}

acquire_lock() {
  LOCK_FILE="$INSTALL_DIR/.OpenMPA.install.lock"
  [ ! -L "$LOCK_FILE" ] || die "installer lock must not be a symbolic link: $LOCK_FILE"
  if [ "$USE_SUDO" -eq 1 ]; then
    /usr/bin/sudo /usr/bin/touch "$LOCK_FILE"
    [ -f "$LOCK_FILE" ] && [ ! -L "$LOCK_FILE" ] ||
      die "installer lock must be a regular file: $LOCK_FILE"
    /usr/bin/sudo /bin/chmod 0666 "$LOCK_FILE"
  else
    /usr/bin/touch "$LOCK_FILE"
    [ -f "$LOCK_FILE" ] && [ ! -L "$LOCK_FILE" ] ||
      die "installer lock must be a regular file: $LOCK_FILE"
  fi

  exec 9>"$LOCK_FILE" || die "could not open installer lock: $LOCK_FILE"
  if ! /usr/bin/lockf -s -t 0 9; then
    exec 9>&-
    die "another OpenMPA installer is active for $INSTALL_DIR"
  fi
  LOCK_HELD=1
}

recover_interrupted_transaction() {
  for transaction_path in "$STAGE_APP" "$BACKUP_APP"; do
    [ ! -L "$transaction_path" ] ||
      die "transaction path must not be a symbolic link: $transaction_path"
  done

  if [ -e "$BACKUP_APP" ]; then
    validate_existing_bundle_id "$BACKUP_APP"
    if [ ! -e "$TARGET_APP" ] && [ ! -L "$TARGET_APP" ]; then
      move_managed_path "$BACKUP_APP" "$TARGET_APP"
      say 'Recovered the previous OpenMPA application from an interrupted installation.'
    else
      [ ! -L "$TARGET_APP" ] || die 'installed OpenMPA target must not be a symbolic link'
      if (validate_app "$TARGET_APP" 'interrupted installed application'); then
        remove_managed_path "$BACKUP_APP"
        say 'Completed cleanup from an interrupted OpenMPA installation.'
      else
        validate_existing_bundle_id "$TARGET_APP"
        remove_managed_path "$TARGET_APP"
        move_managed_path "$BACKUP_APP" "$TARGET_APP"
        say 'Restored the previous OpenMPA application after an interrupted activation.'
      fi
    fi
  fi

  if [ -e "$STAGE_APP" ]; then
    validate_existing_bundle_id "$STAGE_APP"
    remove_managed_path "$STAGE_APP"
    say 'Removed a verified stale OpenMPA staging bundle.'
  fi
}

preflight

TARGET_APP="$INSTALL_DIR/OpenMPA.app"
STAGE_APP="$INSTALL_DIR/.OpenMPA.app.installing"
BACKUP_APP="$INSTALL_DIR/.OpenMPA.app.backup"
TMP_DIR=$(/usr/bin/mktemp -d "${TMPDIR:-/tmp}/openmpa-install.XXXXXX") ||
  die 'could not create a private temporary directory'
TMP_DIR=$(CDPATH= cd -- "$TMP_DIR" && pwd -P)
trap cleanup EXIT HUP INT TERM
acquire_lock
recover_interrupted_transaction
for managed_path in "$STAGE_APP" "$BACKUP_APP"; do
  [ ! -e "$managed_path" ] && [ ! -L "$managed_path" ] ||
    die "transaction path already exists: $managed_path"
done

MOUNT_POINT="$TMP_DIR/mount"
/bin/mkdir "$MOUNT_POINT"

DMG_PATH="$TMP_DIR/$DMG_NAME"
DMG_URL="$BASE_URL/releases/v$VERSION/$DMG_NAME"
say "Downloading OpenMPA v$VERSION..."
/usr/bin/curl --fail --silent --show-error --location \
  --proto '=https' --proto-redir '=https' --tlsv1.2 --output "$DMG_PATH" "$DMG_URL"

actual_dmg_sha=$(/usr/bin/shasum -a 256 "$DMG_PATH" | /usr/bin/awk '{print $1}')
[ "$actual_dmg_sha" = "$DMG_SHA256" ] ||
  die "DMG SHA-256 mismatch (expected $DMG_SHA256, found $actual_dmg_sha)"
/usr/bin/hdiutil verify "$DMG_PATH" >/dev/null || die 'DMG container verification failed'

/usr/bin/hdiutil attach "$DMG_PATH" -mountpoint "$MOUNT_POINT" \
  -readonly -nobrowse -noautoopen -quiet || die 'could not mount the DMG read-only'
MOUNT_ATTACHED=1

APP_LIST="$TMP_DIR/apps.txt"
/usr/bin/find "$MOUNT_POINT" -name 'OpenMPA.app' -prune -print >"$APP_LIST"
app_count=$(/usr/bin/wc -l <"$APP_LIST" | /usr/bin/tr -d ' ')
[ "$app_count" = '1' ] || die "DMG must contain exactly one OpenMPA.app (found $app_count)"
SOURCE_APP=$(/usr/bin/sed -n '1p' "$APP_LIST")
validate_app "$SOURCE_APP" 'source application'

say "Staging OpenMPA in $INSTALL_DIR..."
copy_to_stage "$SOURCE_APP"
validate_app "$STAGE_APP" 'staged application'
if [ "$USE_SUDO" -eq 1 ]; then
  if /usr/bin/sudo /usr/bin/xattr -p com.apple.quarantine "$STAGE_APP" >/dev/null 2>&1; then
    /usr/bin/sudo /usr/bin/xattr -dr com.apple.quarantine "$STAGE_APP"
  fi
elif /usr/bin/xattr -p com.apple.quarantine "$STAGE_APP" >/dev/null 2>&1; then
  /usr/bin/xattr -dr com.apple.quarantine "$STAGE_APP"
fi
validate_app "$STAGE_APP" 'staged application after quarantine removal'

if [ -e "$TARGET_APP" ] || [ -L "$TARGET_APP" ]; then
  validate_existing_bundle_id "$TARGET_APP"
  quiesce_existing_app
  BACKUP_CREATED=1
  move_managed_path "$TARGET_APP" "$BACKUP_APP"
fi

ACTIVATED=1
move_managed_path "$STAGE_APP" "$TARGET_APP"
validate_app "$TARGET_APP" 'installed application'
INSTALL_COMPLETE=1

if [ "$BACKUP_CREATED" -eq 1 ]; then
  remove_managed_path "$BACKUP_APP"
  BACKUP_CREATED=0
fi
say "Installed OpenMPA v$VERSION at $TARGET_APP"

if [ "$OPEN_AFTER_INSTALL" -eq 1 ]; then
  /usr/bin/open "$TARGET_APP" ||
    printf 'OpenMPA installer: installation succeeded, but the application did not open automatically.\n' >&2
fi
