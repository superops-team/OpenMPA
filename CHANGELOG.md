# Changelog

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
