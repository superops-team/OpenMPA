# OpenMPA desktop releases

Version 0.1.0 is a preview for macOS on Apple Silicon. Download the DMG or app ZIP from the GitHub release, compare its SHA-256 digest with `SHA256SUMS`, and open OpenMPA. Credentials and Runtime selection are configured in the native Settings page.

Source and binary versions are both 0.1.0. The release tag must point to the reviewed commit used to build the app. Build the frontend and bindings from that commit using Node 24+, Go 1.27.3 and MyGo 0.6.4:

```sh
npm ci
cd desktop
GOWORK=off GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo build -platform darwin/arm64 .
```

This produces `desktop/dist/darwin-arm64/OpenMPA.app` and the DMG. Build outputs stay out of Git; attach only reviewed artifacts to the release. MyGo builds with `-trimpath`, generates the app icon and signs the local bundle. The preview is ad-hoc signed and has no Apple notarization.

Before publishing, run type checks, lint, format checks, Node tests, Go tests/vet and binding generation. Scan the actual Git index with Gitleaks and check for local credential values without printing them. Scan the clean source snapshot, app contents and release archives too. Do not include `.env` files, shell configuration, key files, account-bound live harnesses, local profile stores or private acceptance data. Keep the existing commit hooks active.

Runtime/Worker source and images are external to this release. No backend image is built or deployed by the desktop release process. Runtime-dependent acceptance gaps are documented in the changelog and published notes; v0.1.0 is marked as a GitHub prerelease.
