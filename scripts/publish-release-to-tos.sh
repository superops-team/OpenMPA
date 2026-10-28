#!/bin/bash

set -euo pipefail

BUCKET='openmpa-downloads'
REGION='cn-beijing'
ENDPOINT='tos-cn-beijing.volces.com'
BASE_URL='https://openmpa-downloads.tos-cn-beijing.volces.com'
REVIEWED_INSTALLER_SHA256='1ec3f6df0008901aa77aabd23649095aa55ed09ab34057f9772f45c24c785d50'
VERSION=''
STAGING_DIR=''
INSTALLER_PATH=''
INSTALLER_SHA256=''
MODE='publish'
TMP_DIR=''
READBACK_INDEX=0

say() {
  printf '%s\n' "$*"
}

die() {
  printf 'OpenMPA TOS publisher: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Validate and publish an existing OpenMPA release to its public TOS mirror.

Usage:
  publish-release-to-tos.sh --version VERSION --staging-dir DIRECTORY [OPTIONS]

Required:
  --version VERSION       Numeric version without the v prefix, for example 0.1.3.
  --staging-dir DIR       Directory containing the four reviewed release assets.

Options:
  --installer FILE        Installer to publish; defaults to scripts/install-macos.sh.
  --installer-sha256 SHA  Required with --installer; reviewed SHA-256 of that file.
  --check-only            Validate local release contracts without publishing.
  --dry-run               Validate and print the exact upload order without publishing.
  -h, --help              Show this help.

The publisher never builds release artifacts. Versioned objects are uploaded with
If-None-Match: *, then anonymously read back. releases/latest.json and install.sh
are the only mutable objects, and install.sh is uploaded last.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || die '--version requires a value'
      VERSION=$2
      shift 2
      ;;
    --staging-dir)
      [ "$#" -ge 2 ] || die '--staging-dir requires a value'
      STAGING_DIR=$2
      shift 2
      ;;
    --installer)
      [ "$#" -ge 2 ] || die '--installer requires a value'
      INSTALLER_PATH=$2
      shift 2
      ;;
    --installer-sha256)
      [ "$#" -ge 2 ] || die '--installer-sha256 requires a value'
      INSTALLER_SHA256=$2
      shift 2
      ;;
    --check-only)
      [ "$MODE" = 'publish' ] || die '--check-only and --dry-run are mutually exclusive'
      MODE='check'
      shift
      ;;
    --dry-run)
      [ "$MODE" = 'publish' ] || die '--check-only and --dry-run are mutually exclusive'
      MODE='dry-run'
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

[ -n "$VERSION" ] || die '--version is required'
[ -n "$STAGING_DIR" ] || die '--staging-dir is required'

validate_version() {
  value=$1
  case "$value" in
    ''|.*|*.|*[!0-9.]*) return 1 ;;
  esac
  old_ifs=$IFS
  IFS=.
  set -- $value
  IFS=$old_ifs
  [ "$#" -eq 3 ] || return 1
  for component in "$@"; do
    [ -n "$component" ] || return 1
  done
}

validate_version "$VERSION" || die 'version must contain exactly three numeric components'
[ -d "$STAGING_DIR" ] || die "staging directory does not exist: $STAGING_DIR"
STAGING_DIR=$(CDPATH= cd -- "$STAGING_DIR" && pwd -P)

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
if [ -z "$INSTALLER_PATH" ]; then
  INSTALLER_PATH="$SCRIPT_DIR/install-macos.sh"
  [ -z "$INSTALLER_SHA256" ] || die '--installer-sha256 is only valid with --installer'
elif [ "${INSTALLER_PATH#/}" = "$INSTALLER_PATH" ]; then
  INSTALLER_PATH="$(pwd -P)/$INSTALLER_PATH"
fi
[ -f "$INSTALLER_PATH" ] && [ ! -L "$INSTALLER_PATH" ] ||
  die "installer must be a regular non-symbolic-link file: $INSTALLER_PATH"
/bin/bash -n "$INSTALLER_PATH" || die 'installer has invalid Bash syntax'
actual_installer_sha=$(/usr/bin/shasum -a 256 "$INSTALLER_PATH" | /usr/bin/awk '{print $1}')
if [ "$INSTALLER_PATH" = "$SCRIPT_DIR/install-macos.sh" ]; then
  [ "$actual_installer_sha" = "$REVIEWED_INSTALLER_SHA256" ] ||
    die 'repository installer does not match its reviewed SHA-256'
else
  case "$INSTALLER_SHA256" in
    ''|*[!0-9a-f]*) die '--installer requires a reviewed lowercase --installer-sha256' ;;
  esac
  [ "${#INSTALLER_SHA256}" -eq 64 ] ||
    die '--installer-sha256 must contain 64 hexadecimal characters'
  [ "$actual_installer_sha" = "$INSTALLER_SHA256" ] ||
    die 'installer does not match the reviewed --installer-sha256'
fi
JQ=$(command -v jq) || die 'jq is required to validate release JSON contracts'

DMG_NAME="OpenMPA-v$VERSION-macos-arm64.dmg"
ZIP_NAME="OpenMPA-v$VERSION-macos-arm64.zip"
SUMS_NAME='SHA256SUMS'
VERIFICATION_NAME='build-verification.json'
DMG_PATH="$STAGING_DIR/$DMG_NAME"
ZIP_PATH="$STAGING_DIR/$ZIP_NAME"
SUMS_PATH="$STAGING_DIR/$SUMS_NAME"
VERIFICATION_PATH="$STAGING_DIR/$VERIFICATION_NAME"

for asset_path in "$DMG_PATH" "$ZIP_PATH" "$SUMS_PATH" "$VERIFICATION_PATH"; do
  [ -f "$asset_path" ] && [ ! -L "$asset_path" ] ||
    die "required asset must be a regular non-symbolic-link file: $asset_path"
done

sha256_file() {
  /usr/bin/shasum -a 256 "$1" | /usr/bin/awk '{print $1}'
}

checksum_for() {
  /usr/bin/awk -v expected_name="$1" '
    $2 == expected_name {
      if (found) exit 2
      print $1
      found = 1
    }
    END { if (!found) exit 1 }
  ' "$SUMS_PATH"
}

/usr/bin/awk -v dmg="$DMG_NAME" -v zip="$ZIP_NAME" '
  NF != 2 { exit 1 }
  length($1) != 64 || $1 ~ /[^0-9a-f]/ { exit 1 }
  $2 == dmg { dmg_count++; next }
  $2 == zip { zip_count++; next }
  { exit 1 }
  END { if (dmg_count != 1 || zip_count != 1 || NR != 2) exit 1 }
' "$SUMS_PATH" || die 'SHA256SUMS must contain exactly the DMG and ZIP SHA-256 entries'

DMG_SHA256=$(checksum_for "$DMG_NAME") || die "missing checksum for $DMG_NAME"
ZIP_SHA256=$(checksum_for "$ZIP_NAME") || die "missing checksum for $ZIP_NAME"
[ "$(sha256_file "$DMG_PATH")" = "$DMG_SHA256" ] || die 'DMG does not match SHA256SUMS'
[ "$(sha256_file "$ZIP_PATH")" = "$ZIP_SHA256" ] || die 'ZIP does not match SHA256SUMS'

"$JQ" -e . "$VERIFICATION_PATH" >/dev/null || die 'build-verification.json is not valid JSON'
json_raw() {
  "$JQ" -er ".$1" "$VERIFICATION_PATH" 2>/dev/null
}

[ "$(json_raw schema)" = 'openmpa-build-verification/v1' ] || die 'unexpected build-verification schema'
[ "$(json_raw version)" = "$VERSION" ] || die 'build-verification version does not match'
[ "$(json_raw tag)" = "v$VERSION" ] || die 'build-verification tag does not match'
[ "$(json_raw platform)" = 'darwin/arm64' ] || die 'build-verification platform must be darwin/arm64'
COMMIT=$(json_raw commit) || die 'build-verification commit is missing'
case "$COMMIT" in
  *[!0-9a-f]*|'') die 'build-verification commit must be a lowercase hexadecimal Git commit' ;;
esac
[ "${#COMMIT}" -eq 40 ] || die 'build-verification commit must contain 40 hexadecimal characters'
[ "$(json_raw artifacts.dmgSha256)" = "$DMG_SHA256" ] || die 'build-verification DMG digest does not match'
[ "$(json_raw artifacts.zipSha256)" = "$ZIP_SHA256" ] || die 'build-verification ZIP digest does not match'
[ "$(json_raw verification.codeSignature)" = 'valid ad-hoc signature' ] ||
  die 'build-verification must record the valid ad-hoc signature'
[ "$(json_raw verification.dmg)" = 'hdiutil verify passed' ] ||
  die 'build-verification must record hdiutil verification'
[ "$(json_raw verification.zip)" = 'unzip integrity test passed' ] ||
  die 'build-verification must record ZIP integrity verification'
[ "$(json_raw dmg.included)" = 'true' ] || die 'build-verification must include the DMG'
[ "$(json_raw dmg.verified)" = 'true' ] || die 'build-verification must mark the DMG verified'

/usr/bin/grep -F "VERSION='$VERSION'" "$INSTALLER_PATH" >/dev/null ||
  die 'installer version does not match the release'
/usr/bin/grep -F "BASE_URL='$BASE_URL'" "$INSTALLER_PATH" >/dev/null ||
  die 'installer public base URL does not match the TOS mirror'
/usr/bin/grep -F "DMG_NAME='$DMG_NAME'" "$INSTALLER_PATH" >/dev/null ||
  die 'installer DMG name does not match the release'
/usr/bin/grep -F "DMG_SHA256='$DMG_SHA256'" "$INSTALLER_PATH" >/dev/null ||
  die 'installer DMG digest does not match the release'

TMP_DIR=$(/usr/bin/mktemp -d "${TMPDIR:-/tmp}/openmpa-publish.XXXXXX") ||
  die 'could not create a private temporary directory'
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    /bin/rm -rf "$TMP_DIR"
  fi
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

LATEST_PATH="$TMP_DIR/latest.json"
cat >"$LATEST_PATH" <<EOF
{
  "schema": "openmpa-latest/v1",
  "version": "$VERSION",
  "tag": "v$VERSION",
  "commit": "$COMMIT",
  "platform": "darwin/arm64",
  "releaseBaseUrl": "$BASE_URL/releases/v$VERSION",
  "dmg": {
    "name": "$DMG_NAME",
    "url": "$BASE_URL/releases/v$VERSION/$DMG_NAME",
    "sha256": "$DMG_SHA256"
  },
  "zip": {
    "name": "$ZIP_NAME",
    "url": "$BASE_URL/releases/v$VERSION/$ZIP_NAME",
    "sha256": "$ZIP_SHA256"
  },
  "sha256sumsUrl": "$BASE_URL/releases/v$VERSION/$SUMS_NAME",
  "buildVerificationUrl": "$BASE_URL/releases/v$VERSION/$VERIFICATION_NAME"
}
EOF
"$JQ" -e . "$LATEST_PATH" >/dev/null || die 'generated latest.json is invalid'

VERSION_PREFIX="releases/v$VERSION"
if [ "$MODE" = 'check' ]; then
  say "Release contracts passed for OpenMPA v$VERSION."
  exit 0
fi

if [ "$MODE" = 'dry-run' ]; then
  say 'Upload plan (no network or TOS mutation performed):'
  say "  immutable: tos://$BUCKET/$VERSION_PREFIX/$DMG_NAME"
  say "  immutable: tos://$BUCKET/$VERSION_PREFIX/$ZIP_NAME"
  say "  immutable: tos://$BUCKET/$VERSION_PREFIX/$SUMS_NAME"
  say "  immutable: tos://$BUCKET/$VERSION_PREFIX/$VERIFICATION_NAME"
  say "  mutable:   tos://$BUCKET/releases/latest.json"
  say "  mutable:   tos://$BUCKET/install.sh (last)"
  exit 0
fi

command -v ve >/dev/null 2>&1 || die 've CLI is required to publish'

upload_object() {
  source_path=$1
  object_key=$2
  content_type=$3
  immutability=$4
  say "Uploading tos://$BUCKET/$object_key"
  if [ "$immutability" = 'immutable' ]; then
    ve ve-tos-cli object upload "tos://$BUCKET/$object_key" \
      --body "$source_path" --region "$REGION" --endpoint "$ENDPOINT" \
      --content-type "$content_type" \
      --if-none-match '*' --quiet
  else
    ve ve-tos-cli object upload "tos://$BUCKET/$object_key" \
      --body "$source_path" --region "$REGION" --endpoint "$ENDPOINT" \
      --content-type "$content_type" --quiet
  fi
}

public_object_status() {
  object_key=$1
  output_path=$2
  /usr/bin/curl --silent --show-error --location \
    --proto '=https' --proto-redir '=https' --tlsv1.2 \
    --output "$output_path" --write-out '%{http_code}' "$BASE_URL/$object_key"
}

compare_readback() {
  source_path=$1
  object_key=$2
  readback_path=$3
  source_sha=$(sha256_file "$source_path")
  readback_sha=$(sha256_file "$readback_path")
  [ "$source_sha" = "$readback_sha" ] || die "anonymous readback digest mismatch: $object_key"
  source_size=$(/usr/bin/wc -c <"$source_path" | /usr/bin/tr -d ' ')
  readback_size=$(/usr/bin/wc -c <"$readback_path" | /usr/bin/tr -d ' ')
  [ "$source_size" = "$readback_size" ] || die "anonymous readback length mismatch: $object_key"
}

readback_object() {
  source_path=$1
  object_key=$2
  READBACK_INDEX=$((READBACK_INDEX + 1))
  readback_path="$TMP_DIR/readback.$READBACK_INDEX"
  status=$(public_object_status "$object_key" "$readback_path") ||
    die "anonymous readback request failed: $object_key"
  [ "$status" = '200' ] || die "anonymous readback failed with HTTP $status: $object_key"
  compare_readback "$source_path" "$object_key" "$readback_path"
}

ensure_immutable_object() {
  source_path=$1
  object_key=$2
  content_type=$3
  READBACK_INDEX=$((READBACK_INDEX + 1))
  probe_path="$TMP_DIR/probe.$READBACK_INDEX"
  status=$(public_object_status "$object_key" "$probe_path") ||
    die "anonymous preflight request failed: $object_key"
  case "$status" in
    200)
      compare_readback "$source_path" "$object_key" "$probe_path"
      say "Verified existing immutable object tos://$BUCKET/$object_key"
      return
      ;;
    404) ;;
    *) die "anonymous immutable-object probe failed with HTTP $status: $object_key" ;;
  esac

  if upload_object "$source_path" "$object_key" "$content_type" immutable; then
    readback_object "$source_path" "$object_key"
    return
  fi

  # Another publisher may have won the If-None-Match race. Accept only an exact
  # public byte-for-byte match; a conflicting object remains a hard failure.
  READBACK_INDEX=$((READBACK_INDEX + 1))
  race_path="$TMP_DIR/race.$READBACK_INDEX"
  status=$(public_object_status "$object_key" "$race_path") ||
    die "immutable upload failed and its readback was unavailable: $object_key"
  [ "$status" = '200' ] || die "immutable upload failed with no readable object: $object_key"
  compare_readback "$source_path" "$object_key" "$race_path"
  say "Verified immutable object created by a concurrent publisher: $object_key"
}

ensure_immutable_object "$DMG_PATH" "$VERSION_PREFIX/$DMG_NAME" 'application/x-apple-diskimage'
ensure_immutable_object "$ZIP_PATH" "$VERSION_PREFIX/$ZIP_NAME" 'application/zip'
ensure_immutable_object "$SUMS_PATH" "$VERSION_PREFIX/$SUMS_NAME" 'text/plain; charset=utf-8'
ensure_immutable_object "$VERIFICATION_PATH" "$VERSION_PREFIX/$VERIFICATION_NAME" 'application/json'

upload_object "$LATEST_PATH" 'releases/latest.json' 'application/json' mutable
readback_object "$LATEST_PATH" 'releases/latest.json'

upload_object "$INSTALLER_PATH" 'install.sh' 'text/x-shellscript; charset=utf-8' mutable
readback_object "$INSTALLER_PATH" 'install.sh'

list_status=$(/usr/bin/curl --silent --output /dev/null --write-out '%{http_code}' \
  --proto '=https' --proto-redir '=https' --tlsv1.2 "$BASE_URL/?list-type=2")
[ "$list_status" = '403' ] || die "anonymous bucket listing must remain forbidden (HTTP $list_status)"
say "Published and anonymously verified OpenMPA v$VERSION."
