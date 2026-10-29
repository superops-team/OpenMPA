# OpenMPA desktop host

OpenMPA uses MyGo 0.6.4, a system WebView and a Go host. The frontend embeds the assets built to `../dist/desktop`. AgentKit MPA Runtime and Worker source are reference-only; the client consumes their existing OpenAPI. Chatting does not require a local service. MPA creation and deletion use a bundled, isolated Python engine launched by Go over private stdin/stdout; users do not install Python or VeADK.

Build tools: Go 1.27.3 with `CGO_ENABLED=0`, Node 24+, the repository npm dependencies, and `uv` for preparing the locked Python 3.12 lifecycle engine. Run frontend commands from the repository root, then Go commands from `desktop/`:

```sh
npm run desktop:engine
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

`Platform`, `Connection`, `Session`, `Workspace`, `Usage`, `Channels`, `Schedule`, `Deployment`, `RuntimeConfig`, `MPALifecycle`, `Artifact`, `Knowledge` and `Host` use typed bindings. Cloud AK/SK/STS and Runtime keys stay outside the WebView; the private lifecycle engine receives only the credentials required for its work. Platform requests use `agentkit/2025-10-30` and STS `sts/2018-01-01`; model eligibility uses Ark metadata. The gateway base path is preserved. Authenticated redirects are rejected. Connection epochs cancel active HTTP bodies and reject late responses.

Settings automatically lists the signed-in account's AgentKit Runtimes tagged `veadk:agent-type=mpa`. Region/project selection, name/ID filtering and refresh precede Runtime selection. Account or directory changes invalidate old rows. Selecting a Runtime revalidates its identity and endpoint and resets application session projections. AgentKit key-auth supplies the default Session subject, so conversations work immediately after binding a Runtime with its key. An explicit business identity remains an optional subject override; channel administrator credentials are configured separately and the server remains the authority.

Region is a closed catalog sourced from AgentKit's published service addresses (`cn-beijing` by default, plus `cn-shanghai`). Each control-plane request uses the selected Region for both its endpoint and signature. OpenMPA stores language, Region, project, selected Runtime and window state in the pure-Go SQLite database at MyGo's user-data path. Optional saved credentials use the existing encrypted vault and profile services; raw credential values never enter WebView projections.

Runtime resources and images use existing OpenAPI. Runtime image updates and updates to its formally associated codex-worker Tool are independent actions with reviewed IDs and original operation receipts. Resource updates support description, CPU, memory, instance bounds and concurrency. Accepted submission, platform readiness and actual image adoption are tracked separately. An unknown outcome is recovered by reading its original receipt without automatically resending the update. The client never builds backend images.

The selected Runtime supplies the deployment context. Each Session is an assistant with its own name, model, AGENTS.md and selected Skills. Assistant settings and the composer share one searchable model picker. The native model catalog intersects Runtime-advertised choices with account activation, selected Ark API Key permissions and exact-version `api.responses.support` metadata; failures remain failures and excluded defaults are not reinserted as choices. Model/MCP updates use If-Match. Run submissions persist original operation/key/content/configuration before sending. Draft receipt revisions preserve edits made while a send is pending. SSE termination or subscription cancellation does not abort an accepted remote run. Projected reasoning and safe execution summaries can be displayed; raw tool arguments and credentials are excluded.

The window starts with a 44px MyGo host title bar with AppKit traffic lights, OpenMPA, sidebar toggle and current location. The 48px shortcut rail and 240px business sidebar also use MyGo's host `ui` renderer. The sidebar contains the Runtime selector, New assistant action and assistant list; hover or keyboard focus exposes exact-ID edit/delete menus. Chat owns the Runtime terminal and Trace actions. Settings, channels, schedules, knowledge, assistant configuration, Resources and Collaboration render in the React WebView. Resource and collaboration panels stay mounted while hidden so navigation preserves unsaved edits; connection identity changes isolate their state. Runtime trace v1.4 and locally archived SSE execution events feed the Trace overlay with measured durations, milestone positions, worker filtering and recorded detail. Missing evidence remains unknown. The client caches snapshots and execution JSONL under user data, cleans expired data periodically and removes associated data after an authorized session deletion.

Host actions use a fixed navigation event with exact Session and Runtime IDs. The React page validates membership before selection; safe ShellState projections include a monotonically increasing revision and the directory connection epoch to reject stale updates. Account and Runtime operations use the same typed Go services as IPC. Titles and lists are bounded, contain no secrets, and use the existing character artwork as small host thumbnails. The host controls support accessible labels, tooltips and keyboard focus. Frequent navigation has no decorative motion; subtle 120ms color feedback follows the installed Emil design guidelines. The React desktop token contract lives in [design/README.md](../design/README.md).

The [desktop identity](branding/README.md) provides editable SVG marks and a 1024px app icon. The native home button embeds the monochrome vector; MyGo packages the color icon for Dock and Finder. The short titlebar divider explicitly centers within the 44px bar.

Unknown task creation, trigger, knowledge import and assistant creation requests retain their exact form and token in a 0600 scoped client intent store. Restart does not submit them automatically. File import/download helpers attach the trusted owning window and reuse existing stream, authorization and atomic save logic.

Design follows the user-selected Apple and Emil Kowalski guidelines from [emilkowalski/skills](https://github.com/emilkowalski/skills). Machine-local agent skills and their installation metadata are excluded from the public source release.

Local workspace storage and draft recovery remain in the Go service, but the React desktop navigation no longer exposes Spaces or Pages. A window close requests a workspace flush and awaits the matching Host.resolveClose decision. External navigation opens in the system browser; the production WebView denies web permissions.

Unit tests and builds are separate from live cloud and desktop acceptance. Actual login, Runtime selection, session conversation/recovery, channel configuration, scheduler execution, knowledge ingestion/search, close handling, packaging and performance must have evidence before release. Private acceptance records and per-run binary/frontend hashes live in ignored `.openmpa/`. See the [current client contract](../prd-spec/features/agentkit-desktop/2026-10-25-existing-api-client-spec.md) and [acceptance ledger](../prd-spec/features/agentkit-desktop/2026-10-25-implementation-baseline.md).

## MPA creation and deletion

Open **Manage Runtimes** from the shortcut rail immediately above the new-conversation “+”, or from Settings → Runtime. The chooser keeps its quick-switch role; management works without a connected Runtime. The page provides account/region/project listing, name/ID filtering, creation, connection, deletion and per-region operation history.

The three-step creation dialog uses Studio's name check, provisioning pipeline and persistent `CreationTasks` journal. It prepares IAM, network, APIG, PostgreSQL, an owned Worker and skill space, optional OpenViking/TOS, pins the default images and verifies readiness. Supported creation regions follow Studio's built-in profile; an unconfigured region displays an unavailable configuration. Closing a dialog leaves accepted tasks running. Cancelling stops subsequent steps and retains created resources. Failed/cancelled tasks retry with their original identity; unconfirmed submissions recover by request ID. Nonsecret drafts are session-scoped; manual OpenViking keys must be entered again after reopening.

Accepted deletions are monitored by Go even after leaving the page; completion clears only the matching current account/region/Runtime and notifies the shell. Deletion reads the Runtime, all sessions and owned-resource metadata before confirmation, binds confirmation to that preview, then rechecks before deleting idle sessions, exclusive managed resources and the Runtime. Active sessions and ambiguous ownership block deletion. Shared databases, Workspaces, networks and APIG remain. A lost deletion receipt stays unconfirmed; a read-only recovery confirms completion only with the exact provider NotFound response and absence from the complete Runtime list. No write is automatically repeated. Deleting the connected Runtime clears the connection and conversation projection.

The engine pins `veadk-python==1.1.19` (Studio #1182) and its complete dependency graph in `mpa-engine/uv.lock`. `npm run desktop:engine` builds `build/mpa-engine.tar.gz` with a relocatable CPython 3.12 interpreter and dependencies. `desktop:dev` and `desktop:build` prepare that resource automatically. MyGo copies it into the app's Resources directory. First use extracts it into a 0700 user-data cache and shows preparation feedback; on slower disks this can take a few minutes. Generated pure-Python SDK service modules load directly from a zip, preserving all modules while avoiding about 27,000 small-file extractions. Packages with local resource files stay unpacked. The archive is about 215 MiB on macOS arm64. Subsequent launches reuse the extracted interpreter.

Go supplies verified account credentials over stdin, rejects stale account epochs and keeps credentials out of IPC, command arguments, journals and logs. The child receives credentials only in memory/environment and is shut down on account changes and app exit. Creation/deletion journals are scoped to the verified principal under user data. The reused nonsecret PostgreSQL bootstrap journal retains Studio's `/tmp/veadk-studio/` path and account/provider-scoped locking. No listening HTTP socket is opened. Python and dependency license files remain in the packaged distribution; see [engine notes](mpa-engine/README.md).

Focused checks (mock cloud services only):

```sh
uv run --project desktop/mpa-engine pytest desktop/mpa-engine/test_engine.py -q
npx vitest run tests/desktop-mpa-lifecycle.test.tsx
# From desktop/, after preparing the archive:
OPENMPA_ENGINE_ARCHIVE="$PWD/build/mpa-engine.tar.gz" GOTOOLCHAIN=go1.27.3 go test -race ./internal/mpalifecycle
# With a separate local Vite instance on port 4184:
node scripts/verify-desktop-lifecycle.mjs http://127.0.0.1:4184
```

These checks do not create or delete real cloud resources. Cloud permissions, paid resource availability and provider-side changes still determine live operation success.
