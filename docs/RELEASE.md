# OpenMPA desktop releases

Version 0.1.5 is a preview for macOS on Apple Silicon. Install it from the public TOS mirror using the [documented installer](INSTALL.md), or download the DMG or app ZIP from the GitHub release and compare its SHA-256 digest with `SHA256SUMS`. Credentials and Runtime selection are configured in the native Settings page.

Source and binary versions are both 0.1.5. The release tag must point to the reviewed commit used to build the app. Build the frontend and bindings from that commit using Node 24+, Go 1.27.3 MyGo 0.6.4 and `uv` (for the locked CPython lifecycle engine):

```sh
npm ci
npm run desktop:engine
cd desktop
GOWORK=off GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo build -platform darwin/arm64 .
```

This produces `desktop/dist/darwin-arm64/OpenMPA.app` and the DMG. Package the bundle as `OpenMPA-v0.1.5-macos-arm64.zip` and publish both packages with `SHA256SUMS` and `build-verification.json`. Build outputs stay out of Git; attach only reviewed artifacts to the release. MyGo builds with `-trimpath`, generates the app icon and signs the local bundle. The preview is ad-hoc signed and has no Apple notarization.

Before publishing, run type checks, lint, format checks, Node tests, Go tests/vet and binding generation. Scan the actual Git index with Gitleaks and check for local credential values without printing them. Scan the clean source snapshot, app contents and release archives too. Do not include `.env` files, shell configuration, key files, account-bound live harnesses, local profile stores or private acceptance data. Keep the existing commit hooks active.

Runtime/Worker source and images are external to this release. No backend image is built or deployed by the desktop release process. Runtime-dependent acceptance gaps are documented in the changelog and published notes; v0.1.5 is marked as a GitHub prerelease.

## Publish the verified assets to TOS

TOS mirrors the existing GitHub Release assets; it never rebuilds them. Place
the reviewed DMG, ZIP, `SHA256SUMS`, and `build-verification.json` in one local
staging directory. Validate their filenames, digests, version, tag, commit,
platform, signature record, and archive verification contract without changing
cloud state:

```sh
scripts/publish-release-to-tos.sh \
  --version 0.1.5 \
  --staging-dir /absolute/path/to/release-assets \
  --check-only
```

Use `--dry-run` to print the exact object order. After TOS authentication is
configured outside the repository, omit both mode flags to publish:

```sh
scripts/publish-release-to-tos.sh \
  --version 0.1.5 \
  --staging-dir /absolute/path/to/release-assets
```

The publisher requires `jq` for strict JSON contract checks and uses
`ve ve-tos-cli object upload`. All four
`releases/v0.1.5/*` objects use `--if-none-match '*'` and therefore cannot
overwrite an existing version object. It anonymously downloads each uploaded
object and compares both byte length and SHA-256. Only then does it generate and
upload mutable `releases/latest.json`; mutable `install.sh` is uploaded and
verified last. Anonymous bucket listing must still return HTTP 403.

The publisher contains no credentials and does not print credential values.
Authentication stays in the operator's `ve`/TOS configuration. Do not enable
shell tracing when publishing. The default repository installer is pinned by a
reviewed SHA-256 inside the publisher; an explicit `--installer` is rejected
unless its reviewed digest is also supplied with `--installer-sha256`.

The preview remains ad-hoc signed and is not notarized. The installer removes
only the `com.apple.quarantine` attribute from its fully verified staged app; it
does not re-sign the release, disable Gatekeeper, or recursively change xattrs
outside that staged bundle.
