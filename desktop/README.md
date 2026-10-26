# OpenMPA desktop host

OpenMPA uses MyGo 0.6.4, a system WebView and a Go host. The frontend embeds the assets built to `../dist/desktop`. AgentKit MPA Runtime and Worker source are reference-only; the client consumes their existing OpenAPI. No local Node, Bun or Python service is required at runtime.

Build tools: Go 1.27.3 with `CGO_ENABLED=0`, Node 24+, and the repository npm dependencies. Run frontend commands from the repository root, then Go commands from `desktop/`:

```sh
npm run build:desktop:frontend
npm run typecheck
npm run lint
npm test -- --maxWorkers=2
cd desktop
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go test ./...
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go test -race ./...
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go vet ./...
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo generate
GOWORK=off GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go build ./...
GOWORK=off GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go run github.com/egoist/mygo/cmd/mygo build -platform darwin/arm64 .
```

An optional ignored `go.work` selects a local MyGo checkout for development. Production dependencies pin MyGo 0.6.4 and Volcengine's official signer 1.0.258; `GOWORK=off` verifies independence from that checkout. Construction and binding do not read credentials or contact the network, so `mygo generate` can exit before window creation.

`Platform`, `Connection`, `Session`, `Workspace`, `Channels`, `Schedule`, `Deployment`, `RuntimeConfig`, `Artifact`, `Knowledge` and `Host` use typed bindings. Cloud AK/SK/STS and Runtime keys stay in Go. Platform requests use `agentkit/2025-10-30` and STS `sts/2018-01-01`. The gateway base path is preserved. Authenticated redirects are rejected. Connection epochs cancel active HTTP bodies and reject late responses.

Settings automatically lists the signed-in account's AgentKit Runtimes tagged `veadk:agent-type=mpa`. Region/project selection, name/ID filtering and refresh precede Runtime selection. Account or directory changes invalidate old rows. Selecting a Runtime revalidates its identity and endpoint and resets application session projections. Business identity and channel administrator credentials are configured separately; the server decides access.

Runtime resources and images use existing OpenAPI. Runtime image updates and updates to its formally associated codex-worker Tool are independent actions with reviewed IDs and original operation receipts. Resource updates support description, CPU, memory, instance bounds and concurrency. Accepted submission, platform readiness and actual image adoption are tracked separately. An unknown outcome is recovered by reading its original receipt without automatically resending the update. The client never builds backend images.

Each assistant uses an existing Session. Configuration reads the Runtime's supported execution-config model/MCP references and updates them with If-Match. Run submissions persist original operation/key/content/configuration before sending. Recovery reads history and pending state for the same invocation. SSE termination or subscription cancellation does not abort an accepted remote run. Event projection excludes raw tool arguments, secrets and reasoning.

The window's first region is a 44px native MyGo title bar with AppKit traffic lights, OpenMPA, sidebar toggle, current location and artifact action. The 48px shortcut rail and 240px business sidebar below it also use MyGo's native `ui` renderer. Assistants, Spaces and recent conversations use native virtual lists; scheduling, knowledge, channels and settings entries stay at the bottom. Collapsing the business sidebar retains the shortcut rail and selected page. Settings, channels, schedules, knowledge, creation dialogs and the artifact panel use native MyGo controls. Conversation configuration, rename and delete confirmation also use native dialogs. Chat, local document editing and rich text remain in the system WebView.

Native actions use a fixed navigation event with exact Session/Space IDs. The page validates membership and unsaved-draft decisions before selection; safe ShellState projections include a monotonically increasing revision and the directory connection epoch to reject stale updates. Native account/Runtime operations use the same typed Go services as IPC. Titles and lists are bounded, contain no secrets, and use the existing character artwork as small native thumbnails. Native controls support accessible labels, tooltips and keyboard focus. Frequent navigation has no decorative motion; subtle 120ms color feedback follows the installed Emil design guidelines.

The [desktop identity](branding/README.md) provides editable SVG marks and a 1024px app icon. The native home button embeds the monochrome vector; MyGo packages the color icon for Dock and Finder. The short titlebar divider explicitly centers within the 44px bar.

Unknown task creation, trigger, knowledge import and assistant creation requests retain their exact form and token in a 0600 scoped client intent store. Restart does not submit them automatically. Native file import/download helpers attach the trusted owning window and reuse existing stream, authorization and atomic save logic.

Design follows the user-selected Apple and Emil Kowalski guidelines from [emilkowalski/skills](https://github.com/emilkowalski/skills). Machine-local agent skills and their installation metadata are excluded from the public source release.

Local Spaces and Pages retain editor, draft recovery and native Markdown export. A window close requests a workspace flush and awaits the matching Host.resolveClose decision. External navigation opens in the system browser; the production WebView denies web permissions.

Unit tests and builds are separate from live cloud and native acceptance. Actual login, Runtime selection, session conversation/recovery, channel configuration, scheduler execution, knowledge ingestion/search, native save/cancel/close, packaging and performance must have evidence before release. Private acceptance records and per-run binary/frontend hashes live in ignored `.openmpa/`. See the [current client contract](../prd-spec/features/agentkit-desktop/2026-10-25-existing-api-client-spec.md) and [acceptance ledger](../prd-spec/features/agentkit-desktop/2026-10-25-implementation-baseline.md).
