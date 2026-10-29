# Install OpenMPA on macOS

OpenMPA v0.1.5 supports Apple Silicon Macs running macOS 13.0 or newer. The
installer downloads the fixed v0.1.5 DMG from the public OpenMPA TOS mirror,
checks its pinned SHA-256 digest and DMG container, and installs
`OpenMPA.app` in `/Applications`.

## One-line install

```sh
/bin/bash -c "set -o pipefail; /usr/bin/curl -fsSL --proto '=https' --proto-redir '=https' https://openmpa-downloads.tos-cn-beijing.volces.com/install.sh | /bin/bash"
```

The command is a convenience entry point. For an auditable installation,
download the script first, compare it with
[`scripts/install-macos.sh`](../scripts/install-macos.sh), inspect it, and run
the reviewed local file:

```sh
curl -fsSL --proto '=https' --proto-redir '=https' \
  -o /tmp/openmpa-install.sh \
  https://openmpa-downloads.tos-cn-beijing.volces.com/install.sh
shasum -a 256 /tmp/openmpa-install.sh
less /tmp/openmpa-install.sh
/bin/bash /tmp/openmpa-install.sh
```

The repository source and its reviewed Git commit remain the audit authority.
A checksum served from the same bucket is not an independent identity proof.

To install in a writable user directory, pass an absolute directory. This is
also useful for isolated acceptance testing:

```sh
/bin/bash /tmp/openmpa-install.sh --install-dir "$HOME/Applications" --no-open
```

The installer itself refuses to run as root. For the default `/Applications`
target, it may ask for administrator approval only for the exact app copy,
rename, cleanup, and staged quarantine operations. A custom install directory
must be writable by the current user and does not use `sudo`.

## Security model

The v0.1.5 preview has a valid ad-hoc code signature but is not Apple-notarized.
That means macOS cannot present a verified Developer ID publisher, and a normal
browser or Finder download may retain a quarantine marker that blocks first
launch.

The installer does not disable Gatekeeper and does not re-sign the app. It:

1. verifies the pinned DMG SHA-256 and the DMG container;
2. mounts the DMG read-only, without browsing or auto-opening it;
3. requires exactly one `OpenMPA.app` and rejects symbolic links anywhere in
   the bundle;
4. validates the source, staged, and installed bundles with strict code-signing,
   Bundle ID, version, and arm64 checks;
5. removes only `com.apple.quarantine`, only from the already-validated staged
   `OpenMPA.app`, then validates the staged app again; and
6. uses same-volume staged and backup renames so a failed replacement can
   restore the previous app.

The installer never uses global `xattr` cleanup, disables Gatekeeper, changes
SIP/AMFI settings, or applies a local ad-hoc re-sign. It does not modify user
data, credentials, Runtime configuration, or Application Support data.

The exact v0.1.5 DMG digest is:

```text
ff0ec5399e4bc52f29f416b08ef0c74d6e71af32b4474188ece12bbdc54bafe0  OpenMPA-v0.1.5-macos-arm64.dmg
```

You can also download the DMG, ZIP, `SHA256SUMS`, and
`build-verification.json` directly from
`https://openmpa-downloads.tos-cn-beijing.volces.com/releases/v0.1.5/` by
appending the exact object name. Directory listing is intentionally disabled.
