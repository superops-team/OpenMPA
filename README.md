# OpenMPA

A native macOS desktop workspace for AgentKit MPA Runtime, built with MyGo. Native navigation and management screens surround a system WebView for conversation and rich document editing.

**v0.1.5 preview · macOS Apple Silicon**

- [Download v0.1.5](https://github.com/superops-team/OpenMPA/releases/tag/v0.1.5)
- [Install on macOS](docs/INSTALL.md)
- [Desktop setup and architecture](desktop/README.md)
- [Release notes and acceptance limits](CHANGELOG.md)
- [Build and release process](docs/RELEASE.md)

Open Settings to sign in, select an MPA Runtime visible to your account, and configure its business identity. Use independent assistants/Sessions, local Spaces, channel settings and server-side schedules. Runtime and worker images can be configured through existing OpenAPI.

Install the preview from the public mirror with:

```sh
/bin/bash -c "set -o pipefail; /usr/bin/curl -fsSL --proto '=https' --proto-redir '=https' https://openmpa-downloads.tos-cn-beijing.volces.com/install.sh | /bin/bash"
```

The preview uses an ad-hoc macOS signature, is not notarized, and is distributed as DMG and app ZIP packages. The installer verifies the pinned release before removing quarantine only from its staged app; see the [installation and security details](docs/INSTALL.md). Runtime-dependent completion, knowledge, dedicated channel write operations, native file dialogs and new image adoption still have acceptance gaps; see the changelog. Account configuration and credentials are never bundled.

OpenMPA builds on the upstream OpenDots editor and character artwork under the MIT license. The [original upstream documentation](docs/UPSTREAM.md) is preserved separately.
