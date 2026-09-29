<div align="center">

# OpenDots

### A little dot. A lot off your plate.

**An open-source template for persistent AI coworkers.**

Spaces, specialist agents, and conversations that move between text, calls, and Slack.

[Get started](#get-started) · [Overview](#overview) · [Architecture](#architecture) · [Status](#development-status) · [Contributing](CONTRIBUTING.md)

</div>

---

![Live specialist conversation](docs/demos/specialist-chat.gif)

_A live conversation with Scout, with the updated Dot avatars and separate Dots, Spaces, and recent chats. [Watch the MP4](docs/demos/specialist-chat.mp4)._

## Overview

OpenDots is a starting point for building your own agent workspace. Clone it, define your Dots, connect your services, and adapt the interface and tools to your needs.

**A template, not a hosted product.** You run the application and configure its infrastructure. The template is in early development; the status table below distinguishes local verification from connected-service testing.

### Spaces

A Space is a home for working documents. Dots appear separately in navigation and can be granted access to multiple Spaces in their settings. Each Dot has a default destination for saved pages; existing installations retain their original Space access. Browse pages in a searchable library, switch between grid and list views, and organize documents as nested subpages. Open a page in a focused visual editor with formatting, slash commands, and undo/redo. Write directly, save a conversation as a page, or ask a specialist to create and revise content.

Pages stay in the local workspace database. Their conversations use CopilotKit Threads, with a separate conversation for each page and specialist. Page links connect the document workspace to Dot chat. Manual editing works before you configure conversation services. Autosave reports its progress, failed saves retain your draft, and revision checks prevent stale edits from overwriting newer content. Markdown source mode remains available.

![Navigate Spaces and ask a specialist about a page](docs/demos/spaces-page-chat.gif)

_Open a Space, navigate to its launch brief, ask Scout about the saved page, and continue in Dot chat. This recording uses live page chat and example launch content. [Watch the MP4](docs/demos/spaces-page-chat.mp4)._

### Specialist Dots

Give each Dot a name, role, instructions, and permitted tools. A researcher can investigate a topic; a writer can turn findings into a draft. Inspect their work and control what they can do.

### Dot computers

Each Dot can have its own computer, using [OpenBot](https://github.com/CopilotKit/OpenBot)'s container supervisor and computer service. Its browser profile and workspace files persist across stop/start. The Computer panel exposes browser control, human takeover, files, terminal output, and activity, with browser, file, and shell permissions set per Dot. The application keeps service credentials on the server and derives a different computer credential for each Dot.

See [Computer setup](docs/COMPUTERS.md) to build the pinned services and connect your deployment. Computer tools require those services; an unconfigured template does not execute commands on your host.

![A Dot browsing and saving notes through natural-language chat](docs/demos/computer-chat.gif)

_Ask Scout to open a website, summarize it, save notes, and verify the file. Every computer action in this demo is requested through chat; CopilotKit tool renderers show the live browser, saved file, and terminal output inline. [Watch the MP4](docs/demos/computer-chat.mp4)._

### Text and calls

A continuous conversation keeps the Dot's avatar and status above the messages, with text and call controls close at hand. Work updates, source links, and call receipts appear in the timeline; a side panel shows results or the agent's computer.

Calls pair realtime speech with a separate compute agent, so the conversation can continue while longer work runs. Both use the same conversation context and tool permissions. Voice needs separate provider configuration.

### Slack

Mention a Dot through a managed Slack connection using Channels SDK, then continue in its thread. The integration follows [OpenTag](https://github.com/CopilotKit/OpenTag), with an explicit workspace/user allowlist and a selected specialist. See [Slack setup](docs/SETUP.md#slack) to connect your deployment.

## Architecture

The template uses CopilotKit's React SDK and runtime, Intelligence for durable Threads, and Channels SDK for Slack. Pages, application metadata, and background-work state are stored separately from conversation history.

```mermaid
flowchart TB
  Web["Web app: pages, Spaces, Dots, chat"] --> Runtime[CopilotKit runtime]
  Slack[Slack] <--> Managed[Managed channel connection]
  Managed <--> Channels[Channels SDK]
  Channels --> Agents[Specialist compute agents]
  Runtime --> Agents
  Runtime <--> Intelligence["Intelligence / Threads"]
  Channels <--> Intelligence
  Web <--> Speech[Realtime speech]
  Speech --> Bridge[Compute bridge]
  Bridge --> Agents
  Agents --> Controls[Tool permissions]
  Controls --> Computer[Isolated browser / workspace]
  Agents --> Jobs[Background work]
  Runtime --> Metadata[(Pages, Spaces, Dots, work metadata)]
```

You configure the Intelligence project, model provider, and channel connection for your deployment; calls also need a speech provider. Credentials stay on the server. Missing configuration should produce a clear setup state, and test fixtures should remain visibly separate from live integrations.

[OpenMuse](https://github.com/CopilotKit/OpenMuse) and [OpenBot](https://github.com/CopilotKit/openbot) are code references for persistent work, agent computers, and execution controls. OpenDots can be adapted to your own workflows and deployment choices.

## Get started

Use **Node.js 24** and **npm**:

```sh
git clone https://github.com/CopilotKit/OpenDots.git
cd OpenDots
npm ci
cp .env.example .env
npm run dev
```

Open **http://127.0.0.1:5173**. You can create Spaces, write pages, and configure Dots before connecting services. Add your conversation and model settings to `.env` to start chatting.

See [Setup](docs/SETUP.md) for configuration, Slack, calls, the browser service, and Docker.

## Development status

| Area                       | Included                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------- |
| Spaces and Specialist Dots | Saved names, role instructions, and per-Dot research and memory permissions                       |
| Pages                      | Searchable library, visual editor, slash commands, autosave, and revision checks                  |
| Conversations              | React SDK chat and Threads integration, page-specific conversations, and source links             |
| Slack                      | Managed Channels SDK declaration with workspace and user allowlists                               |
| Calls                      | WebRTC speech, delegated compute, bounded sessions, hangup, and timeline receipts                 |
| Background work            | Scheduled server-side turns in their original conversation, with pause and retry controls         |
| Browser                    | Separate read-only public-page service with page capture and navigation limits                    |
| Dot computers              | Per-Dot browser profiles, files, shell, takeover, permissions, and action records through OpenBot |
| Memory                     | User-managed preferences that permitted Dots can use                                              |
| Deployment                 | Local Node setup and separate application/browser containers                                      |

Local checks cover setup, persistence, permissions, SDK failure handling, and browser isolation. Automated tests use service fixtures. **Live Intelligence, model responses, and page-context chat were verified on September 29, 2026.** Live OpenBot computer browsing, file creation, shell verification, and file persistence across stop/start were also verified locally. Slack and voice still require their own connected-service verification. See [recording notes](docs/demos/README.md) for the demonstrated flows and limits.

This is a single-owner starting point. Shared editing, invitations, file uploads, and interactive page embeds are not included. Schedules are recurring instructions, not a complete goal or event-trigger system. Specialist Dots have separate roles and conversations; multi-Dot group conversations and automatic delegation are further work.

### Extending the template

- Add identity, Space membership, and shared page editing for multi-user deployments.
- Add file attachments and richer page content.
- Add event triggers and a persistent responsibility lifecycle.
- Extend tools and approval flows for your own workflows.
- Add richer artifacts, connected-app context, and specialist coordination.

## Contributing

See [Contributing](CONTRIBUTING.md) for development guidance and [Security](SECURITY.md) for reporting issues. Contributions should describe the workflow they enable, include verification evidence, and distinguish live integrations from fixtures.

## References

- [CopilotKit documentation](https://docs.copilotkit.ai/intelligence/overview)
- [Channels SDK](https://github.com/CopilotKit/channels-sdk)
- [OpenMuse](https://github.com/CopilotKit/OpenMuse)
- [OpenBot](https://github.com/CopilotKit/openbot)
- [OpenTag](https://github.com/CopilotKit/OpenTag) — Channels SDK integration reference

## License

[MIT](LICENSE).
