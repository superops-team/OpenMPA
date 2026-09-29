# Recorded walkthroughs

Recorded from the local OpenDots app on September 29, 2026. The recordings use a dedicated `opendots` Intelligence project, a live `gpt-5.4-mini` model, and example content in a separate Launch studio Space. No customer data or credentials are shown.

| Recording                                    | Flow                                                                                          | Duration   |
| -------------------------------------------- | --------------------------------------------------------------------------------------------- | ---------- |
| [Specialist chat](specialist-chat.mp4)       | Continue a launch-planning conversation with Scout and receive a live follow-up response      | 16 seconds |
| [Spaces and page chat](spaces-page-chat.mp4) | Open a Space and page, ask about the saved brief, then open the same conversation under Scout | 24 seconds |

The recordings show the updated plush avatars and separate Dots, Spaces, and recent-chat navigation. Space-access settings preserve existing grants. The computer demo uses Scout with browser, file, and shell access enabled for its own container.

The README embeds compact GIF versions. MP4 versions are included for playback and reuse. These are screen-capture walkthroughs, with pauses between recording segments omitted; no assistant responses are fabricated or replaced. The captures are sampled at four frames per second, so they are intended to show workflows rather than animation performance. They have no audio.

## Chat-driven computer demo

[Computer chat](computer-chat.mp4) (42 seconds) uses the existing CopilotKit `useAgent` / `useCopilotKit` chat and server tools. The OpenBot computer is provisioned before recording. Only these two natural-language messages drive the workflow:

1. “Open https://www.copilotkit.ai in your computer and tell me what it offers in two short bullets.”
2. “Save those notes as copilotkit-notes.md on your computer. Use your terminal to verify the file, then tell me where it is and how many words it contains.”

Scout navigates, summarizes the page, writes the file, and executes a terminal check. Its real reply reports `/workspace/copilotkit-notes.md` and 41 words; an independent container command confirmed both. No manual browser navigation, file editing, or terminal command entry appears in this recording. CopilotKit `useRenderTool` and `CopilotChatToolCallsView` render the computer directly inside the conversation: a live browser card, a file receipt, and terminal output. The side panel stays closed. Tool results and responses are not scripted.

The final recording was captured after correcting host-to-supervisor networking and documenting snapshot recovery following a computer restart. Earlier failed takes are not part of the clip. Between the two chat requests, recording pauses were omitted; response generation within the captured segments is shown as recorded.

## What was verified

- The provided model credential authenticated successfully.
- A dedicated hosted Intelligence project and project-scoped runtime key were provisioned through the CopilotKit CLI.
- Specialist chat returned a live model response.
- The page assistant received the saved document context and returned relevant milestones.
- The launch brief remained saved after a browser reload.
- The page conversation and its model response reloaded from Intelligence after refreshing the browser.

- Live OpenBot computer provisioning, browser navigation, file creation, and shell execution succeeded.
- A saved workspace file retained identical contents across a stop/start (checked separately from the recording).
- Browser work resumed after restart using a fresh snapshot.

Slack and realtime calls are not demonstrated. Their service configuration and live checks remain separate.

## Re-recording

Use a separate example Space and specialist. Verify the model and Intelligence connection before recording. Keep Settings, environment files, credential pages, and unrelated windows out of the capture. Record the actual app, inspect every segment, and update the verification date and scope when replacing the assets. Never replace failed or missing responses with scripted output.
