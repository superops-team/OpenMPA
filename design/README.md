# OpenMPA workbench design tokens

The current contract is [DESIGN.md](../DESIGN.md), backed by
`react-desktop-v2.tokens.json` and validated against `tokens.schema.json`.
React and the native MyGo titlebar, rail and sidebar consume the same values.
`react-desktop-v1.tokens.json` is a historical snapshot, not a second authority.

## Source and scope

The token values come from the existing OpenMPA React surface plus the Codex
composer reference used for the chat input. They are not a copy of Codex CSS.
The accent stays OpenMPA teal. The neutral canvas, surfaces, borders and text
follow the calm, dense Codex-like workbench seen in the reference screenshots.

`react-desktop-v2.tokens.json` owns Light/Dark semantic palettes, type and font
roles, spacing, radius, control and icon sizes, borders, stacking layers, motion,
elevation and layout dimensions. Each status color has a matching foreground.
Media canvases stay white in either theme to preserve QR readability.

`npm run design:generate` validates the source and emits CSS variables, Go
`design.Palette` and dimension constants, and the JSON schema. `npm run
design:check` rejects drift in all three outputs, missing roles, poor contrast,
invalid migrated CSS references and incorrect import order. Do not edit generated
files. The source schema and contrast validation live in `src/design/tokens.ts`.

## Interaction rules

The sidebar owns navigation. Clicking Settings replaces the conversation list
with the settings sections: Account, Runtime, Authentication, Deployment, Theme
and Language. Clicking a section changes only the settings content. It does not
open a second tab strip inside the content.

The Home button at the top of the sidebar returns to chat and expands the sidebar
when it is collapsed. It never destroys the selected conversation.

The Runtime selector is a dropdown. Its assistant list contains independent
conversations with their own name, model, AGENTS.md and Skills. New assistant
is a single action below the selector and requires a selected Runtime. Each
row exposes an Edit/Delete menu on hover or keyboard focus.

Knowledge and scheduled tasks are scoped to the selected Runtime and assistant.

## Motion rules

Press feedback is immediate. Hover and selected states use 150ms color
transitions. Overlays use 180ms. Frequent navigation has no
entrance animation. Dialogs use opacity only and keep stable geometry across
loading and tab changes. Reduced motion removes animation and keeps the state change.

Animate color, opacity and transform. Do not animate layout height, width or
position on every keystroke. Streaming text may update per frame but must not
add per-token fade-in.

## Accessibility

Focus uses a visible ring with at least 3:1 contrast. Body text and placeholders
reach 4.5:1 on their surfaces. Selection is not expressed by color alone. The
settings section list is a keyboard-navigable list. The composer model picker is
a button with an expanded state and Escape closes it.

## Verification

The contract is checked by token/schema negative controls, component tests and
native rail pixel tests that read expected colors directly from the JSON source.
Native themes honor high contrast and text-scale preferences. Visual screenshots are required
before release because static tests cannot prove spacing, focus visibility or
native compositing.

For the production React page matrix, start `npm run dev:desktop:frontend --
--port 4173` without `VITE_OPENMPA_FIXTURE`, then run `npm run design:verify`.
The verifier launches installed Chrome with an isolated profile and provides a
simulated MyGo transport; it renders the real DesktopApp and service adapters.
It covers seven Settings sections, Channels, Schedules, Knowledge, Chat and Trace
in both themes at 1280, 768 and 320px. Each run retains screenshots, the source
digest, errors and computed primary-action colors under ignored
`.openmpa/design-verification/`. These checks do not prove cloud operations or
replace MyGo system WebView acceptance.
