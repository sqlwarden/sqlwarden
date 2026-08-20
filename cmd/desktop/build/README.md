# Desktop build assets

`appicon.png` is rendered from `frontend/public/favicon.svg` and is the canonical native application icon. Wails packages it as the macOS Dock icon and uses the generated `windows/icon.ico` for Windows title-bar and taskbar branding.

When the product mark changes, regenerate both files from the same SVG so native and web branding stay in sync.
