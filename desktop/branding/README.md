# OpenMPA desktop identity

The open ring and separate node form the OpenMPA mark. Graphite `#303832` and sage `#739873` sit on a light `#F3F4EF` tile. The native navigation rail uses the same geometry in one color; assistant character artwork is unchanged.

- `mark.svg`: color mark on a transparent canvas.
- `logo.svg`: mark and OpenMPA wordmark; editable SVG with a system font stack.
- `app-icon.svg`: editable 1024px icon with transparent margins and a rounded tile.
- `app-icon.png`: 1024px sRGB source for MyGo's standard macOS ICNS packaging.
- `preview.png`: icon, wordmark and small-size reference sheet.
- `../internal/host/assets/brand/mark.svg`: generated monochrome SVG embedded by the native shell.

Regenerate the SVG sources and production PNG on macOS:

```sh
swift desktop/branding/generate.swift
```

`mygo.json` selects `branding/app-icon.png` using the top-level `icon` option. MyGo generates `AppIcon.icns` and the bundle declaration; no external image service is required. The brand does not indicate connection or Runtime readiness.
