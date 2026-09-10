# Desktop build assets

`appicon.svg` contains the full-size white-on-brand-blue SQLWarden app icon. `appicon.png` places that artwork at the standard 824px macOS footprint on a transparent 1024px canvas. `icon.png` retains the full-size artwork as Wails' source for file-association icons, and `windows/icon.ico` provides the Windows title-bar, taskbar, and installer icon.

When the product mark changes, regenerate all platform assets from this SVG, retaining the transparent padding only in `appicon.png`. Every `iconName` in `wails.json` must name a PNG in this directory because Wails reads that PNG before producing the platform-specific association icon.
