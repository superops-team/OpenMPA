# Changelog

## 0.1.5 — 2026-10-29

### Features

- Add account-eligible OpenAI Responses-compatible model discovery for assistant creation, assistant settings and the chat composer, with shared searchable pickers and in-memory catalog reuse.
- Cache execution trace history and normalized SSE records locally so finished runs remain visible after Runtime rebuilds or short-lived remote retention.
- Add local prefetch and verification harnesses for Runtime selection stability, chat send interactions, recorded traces, assistant lifecycle and native WebKit model selection.

### Fixes

- Keep Runtime selection visually stable while binding succeeds or fails; success/error feedback no longer shifts the page and localized action labels no longer resize the row.
- Clear the chat composer on the first rendered frame after send, preserve later user edits across late receipts, and remove the distracting send/stop pulse animation.
- Settle Working steps when reasoning, tools and terminal events finish; final answers remain separate from process rows, and completed live processes no longer collapse unexpectedly.
- Replace the native sidebar's one-frame insert/remove jump with an interruptible slide that respects reduced motion and keeps the WebView identity stable.
- Normalize trace step status, timing, command summaries, worker events, URL contrast, input focus rings and recorded worker metrics across light/dark layouts.
- Rename OpenDots product references to OpenMPA in code, docs and telemetry assets.

### Known limitations

- The macOS application remains ad-hoc signed and is not Apple-notarized. The GitHub prerelease provides DMG and app ZIP packages with `SHA256SUMS` and `build-verification.json`.
- Trace TLS-log mirroring still requires Runtime topic configuration that is not bundled with this desktop release.

## 0.1.4 — 2026-10-28

### Features

- Add a chat-scoped native Runtime terminal window that opens through the selected conversation and keeps temporary webshell endpoints inside the Go host.
- Reuse the MyGo native terminal engine for Runtime webshell sessions, including terminal resize, keyboard handling, ANSI rendering and safe connection cleanup.

### Fixes

- Align chat toolbar actions so Terminal and Trace use the same icon-and-label button style as Settings, Rename and Delete.
- Disable Terminal and Trace consistently when no Runtime is connected or no conversation is selected.
- Remove duplicate Trace and Settings entry points from the native title bar and sidebar, keeping those actions in their primary locations.

### Known limitations

- Live Runtime terminal attach depends on the selected account's VE FaaS webshell authorization and a running or explicitly warmed Runtime instance.
- The macOS application remains ad-hoc signed and is not Apple-notarized. The GitHub prerelease provides DMG and app ZIP packages with `SHA256SUMS` and `build-verification.json`.

## 0.1.3 — 2026-10-28

### Features

- Establish a project-wide semantic design-token system for light and dark appearances, with shared React primitives for buttons, dialogs, tabs, disclosures, composers, code blocks, Markdown tables and status rows.
- Add a Turn process view with safe Markdown rendering, structured execution summaries and a detailed trace panel for tool calls, service events and recovered runs.
- Move Assistant Settings into an accessible portal dialog with dedicated AGENTS.md and Skills tabs, and support model selection for the next Turn without rewriting an active Turn.
- Cache Runtime directories per account and project authoritative pending and selected states consistently across Settings, assistant creation and ordinary Session creation.

### Fixes

- Reconcile terminal, Stop and subscription state so completed, stopped and recovered runs release the composer promptly and do not leave the Session permanently unusable.
- Align Assistant Settings, Runtime selection, text inputs, focus rings, selected borders, responsive Skills layouts and Markdown disclosure motion with the shared design language.
- Bind ordinary Session creation to the selected MPA Runtime instead of allowing page-local Runtime state to drift.
- Preserve keyboard access, reduced-motion behavior and usable layouts down to a 320px viewport.

### Known limitations

- AgentKit Runtime v51 can persist a canonical final answer while suppressing the SSE completion event when it returns `status=running should_publish_done=False`. The desktop client recovers terminal state where it has authoritative evidence, but this release does not deploy the Runtime backend fix.
- The macOS application remains ad-hoc signed and is not Apple-notarized. The GitHub prerelease provides both DMG and app ZIP packages with `SHA256SUMS` and `build-verification.json`.

## 0.1.1 — 2026-10-26

### Features

- Add Chinese and English desktop localization, following the macOS language by default.
- Persist non-sensitive desktop preferences and window state in a pure-Go SQLite store. Store optional cloud credentials in a passphrase-encrypted local vault instead of a machine-bound Keychain entry.
- Add account-scoped AgentKit Region, Project and Runtime discovery. Restore a saved Runtime only when it belongs to the current account directory; otherwise keep a visible Runtime selector without projecting another account's Runtime.
- Redesign the conversation surface with a quiet 760px reading column, compact tool activity, an automatically growing composer and explicit jump-to-latest behavior.

### Fixes

- Allow AgentKit Runtime-key identity to create and continue independent Sessions without requiring an unnecessary business-identity override.
- Keep Runtime selection, authentication and deployment tabs synchronized across connection epochs, and add native selected-state keyboard navigation for Settings tabs.
- Deduplicate optimistic user messages, streaming assistant chunks and terminal history projection.
- Preserve editable credential drafts after errors and remove the broken Keychain save/restore path.
- Move the conversation scrollbar to the outer viewport gutter so it no longer overlaps right-aligned user messages.

### Known limitations

- A Session created without an execution-config revision inherits Runtime defaults. The current assistant-settings dialog reports this state as non-editable; per-Session AGENTS.md and Skill editing remains planned for a later release.
- The macOS application remains ad-hoc signed and is not Apple-notarized. Full knowledge and dedicated channel write acceptance still depends on the selected Runtime configuration.

## 0.1.0 — 2026-10-26

First OpenMPA desktop preview for macOS on Apple Silicon, built with MyGo and the existing AgentKit OpenAPI.

- Native macOS title bar, collapsible workspace navigation, assistant thumbnails, settings, channel forms, scheduling, knowledge forms and artifact panel. Conversation and rich text use the system WebView.
- Account sign-in, filtered MPA Runtime discovery and switching. Credentials remain in the Go host with optional system credential storage.
- Conversation history, streaming replies, independent Sessions, execution configuration and recovery of uncertain submissions.
- Server-side scheduled tasks with create/edit, pause/resume, manual runs, history and deletion. Scheduled execution continues after the desktop app exits.
- Local Spaces, rich document editing, draft recovery and Markdown export through the native save flow.
- Runtime resource configuration and independent Runtime/codex-worker image update controls through existing OpenAPI. The client does not build backend images.
- OpenMPA vector logo and macOS app icon, with a centered title-bar separator.

### Preview limitations

Core conversation, Session management, local document persistence and scheduled execution have live acceptance evidence. Complete knowledge operations require a correctly configured Runtime; dedicated channel write fixtures, native file picker/save acceptance and adoption of newly configured images still need end-to-end verification. These modules are included, but complete release acceptance remains open.

The macOS app uses an ad-hoc signature and is not Apple-notarized. This release supplies a DMG, app ZIP and SHA-256 checksums for macOS arm64. Account-specific configuration, environment files, credentials and private acceptance logs are excluded.

OpenMPA retains the original OpenDots editor and character artwork, with upstream MIT attribution.
