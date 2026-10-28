---
version: alpha
name: OpenMPA Workbench
description: Calm, dense and operational desktop UI for long-running agent work.
colors:
  primary: '#2F746B'
  secondary: '#F0F0F0'
  tertiary: '#5F8F87'
  neutral: '#F8F8F8'
typography:
  ui-xl:
    fontFamily: system-ui
    fontSize: 18px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: '-0.01em'
  ui-lg:
    fontFamily: system-ui
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: '-0.01em'
  ui-base:
    fontFamily: system-ui
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: 0em
  ui-sm:
    fontFamily: system-ui
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: 0em
  ui-xs:
    fontFamily: system-ui
    fontSize: 10px
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: 0em
rounded:
  sm: 4px
  md: 6px
  lg: 8px
  xl: 12px
  dialog: 16px
spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
  2xl: 32px
components:
  surface-base:
    backgroundColor: '{colors.neutral}'
    textColor: '#292929'
    rounded: '{rounded.xl}'
    padding: 16px
  button-primary:
    backgroundColor: '{colors.primary}'
    textColor: '#FFFFFF'
    typography: '{typography.ui-base}'
    rounded: '{rounded.md}'
    height: 28px
    padding: 8px
  button-primary-hover:
    backgroundColor: '#255F58'
    textColor: '#FFFFFF'
    typography: '{typography.ui-base}'
    rounded: '{rounded.md}'
    height: 28px
    padding: 8px
  input-default:
    backgroundColor: '#FFFFFF'
    textColor: '#292929'
    typography: '{typography.ui-base}'
    rounded: '{rounded.lg}'
    height: 36px
    padding: 12px
---

## Overview

OpenMPA is a desktop-first AI workbench. The interface is calm, dense and operational. It must support long sessions, readable chat and tool output, keyboard workflows, internationalization, light and dark themes, and native MyGo chrome.

This file is the repository-wide design policy. It adapts the intent of [ZCode DESIGN.md](https://github.com/zai-org/ZCode/blob/3c5b227649371507ddc4eb46ac5133afe6db1cab/DESIGN.md), licensed under Apache-2.0, while retaining OpenMPA's teal identity. Numeric Light and Dark theme values live only in `design/react-desktop-v2.tokens.json`. `src/client/styles/design-tokens.css` is generated and must not be edited by hand.

New UI code must use semantic tokens and the local primitives in `src/client/ui/`. Native MyGo uses generated `desktop/internal/design/tokens_gen.go` through its theme adapter. Do not add a direct dependency on the private `@zcode/ui` workspace package. `design/react-desktop-v1.tokens.json` is historical.

## Colors

- Use semantic roles such as background, panel, surface, card, popover, input, foreground and border. Components never consume raw palette values.
- Use OpenMPA teal only for brand emphasis, the one primary action in a view, active indicators and interactive links. Do not fill large structural surfaces with brand color.
- User messages use a neutral selected surface. Assistant answers remain unboxed reading content.
- Success, warning, destructive and progress each use their own solid, surface and foreground roles. Do not borrow a status hue for decoration.
- Light and Dark themes define the same role set. Components must not derive Dark values.
- All body and control text pairs meet WCAG AA. State surfaces use their matching foreground token.
- Solid status actions use their own foreground, never the primary action foreground. Disabled controls use disabled roles. QR media canvases retain a white background in both themes.

## Typography

- Application UI uses only `ui-xl`, `ui-lg`, `ui-base`, `ui-caption`, `ui-sm` and `ui-xs` roles.
- The default UI size is 14px. UI copy rarely falls below 12px. The 10px role is restricted to badges, counters and weak metadata.
- Mobile Web editable controls use the fixed 16px `mobile-input-safe` role.
- Markdown H1 uses `ui-xl`, H2 uses `ui-lg`, and H3 through H6 use `ui-base` with descending weight.
- Paths, commands, code, hashes and IDs use the mono family. Dynamic numbers use tabular figures.
- Body line-height is 1.5 to 1.6. Long reading text stays within 60 to 75 characters.
- Interface font scaling updates `--ui-font-size`. It never changes the root HTML font size.

## Layout

- MyGo owns the titlebar, rail and sidebar structure. React content does not add a decorative outer card inside that frame.
- Chat transcript and composer share one reading column with a 760px maximum. The native shell owns conversation identity; React Chat does not repeat it in a floating header.
- The transcript owns scrolling. The composer stays in stable chrome.
- Use the 4, 8, 12, 16, 24 and 32px spacing steps. Group with space before adding separators.
- Use logical inline properties for localizable layout.
- Reflow at 320px and tolerate 200% zoom. Primary actions stay reachable.
- Tool and trace rows stay compact. Hidden detail always has a visible disclosure cue.

## Elevation & Depth

- Base and ordinary content surfaces use background contrast and borders, not shadows.
- Cards use a quiet surface and border. They sit above the page and below overlays.
- Menus, popovers and dialogs may use the overlay shadow token.
- Stacking order uses `layer-base`, `layer-sticky`, `layer-popover`, `layer-overlay` and `layer-toast`. Do not invent per-page z-index scales.
- Toasts and rare attention surfaces may use the attention shadow token.
- Do not add large soft shadows to layout frames, chat content or tool rows.

## Shapes

- Container radius starts at 12px. Nested cards step down to 8px, controls to 6px and small tags to 4px.
- Composer shells and dialogs may use 16px as explicit exceptions.
- `radius-full` is reserved for circles and true pills.
- Nested elements never equal or exceed the nearest rounded parent unless they form an independent overlay.
- Default button height is 28px. Compact buttons are 24px. Inputs are 36px. Pointer targets never fall below 24x24px.

## Components

- Use the local `Button`, `IconButton`, `Disclosure`, `StatusRow`, `CodeBlock`, `MarkdownTable` and `ComposerShell` primitives before building a new variant.
- Primitives depend only on React, Lucide and generated CSS tokens. They do not read Runtime, Session, Trace, locale stores or business context.
- Business components own data and state. Primitives own semantics, geometry and visual states.
- Turn process and Trace rows use visible text plus icons for state. Color is never the only signal.
- High-frequency interactions use color and opacity transitions of 150ms or less. Press feedback is 120ms and respects reduced motion.
- Full-width disclosure rows never use scale press feedback. Their content changes immediately; only the chevron may use the 150ms state transition, and keyboard or reduced-motion input changes it instantly.
- Multiline action rows use the shared content-sized Button role and clamp supporting copy to two lines. They never override a fixed-height Button locally.
- Peer navigation, tabs and frequently used view switches do not slide or animate layout. Occasional overlays may enter with the shared 180ms opacity and scale transition.
- Motion uses explicit `transform` and `opacity` properties. Do not animate disclosure height, inherited CSS variables, large shadows or unspecified `all` properties.
- Theme changes suppress transitions for one frame so the application does not smear between palettes.
- Native controls use the same generated role values. High contrast strengthens native text and borders; reduced motion suppresses press transitions.

## Do's and Don'ts

Do:

- use semantic tokens and generated CSS variables;
- preserve distinct background, structural, content and overlay layers;
- keep controls compact and keyboard accessible;
- use text hierarchy before adding boxes and separators;
- verify Light, Dark, 320px, desktop width and real MyGo composition;
- keep Apache-2.0 provenance when source code is adapted.

Don't:

- add raw one-off colors, font sizes, radii or shadows to migrated UI;
- import `@zcode/ui` or its workspace services;
- add Tailwind, Radix or a global state library only to reuse a visual primitive;
- style ordinary content as floating cards;
- use teal, success, warning or destructive colors outside their semantic roles;
- create a generic mega-component that mixes data access with presentation.
