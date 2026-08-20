# Desktop build assets

`appicon.svg` contains the native white-on-brand-blue variant of the SQLWarden mark. `appicon.png` is its 1024px rendered form. Wails packages it as the macOS Dock icon and uses the generated `windows/icon.ico` for Windows title-bar and taskbar branding.

When the product mark changes, regenerate the PNG and Windows icon from this SVG so native and web branding stay in sync.
