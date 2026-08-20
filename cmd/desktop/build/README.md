# Desktop build assets

`appicon.svg` contains the full-size white-on-brand-blue SQLWarden app icon. `appicon.png` places that artwork at the standard 824px macOS footprint on a transparent 1024px canvas. The separately generated `windows/icon.ico` keeps the full-size artwork for Windows title-bar and taskbar legibility.

When the product mark changes, regenerate both platform assets from this SVG, retaining the transparent padding only in `appicon.png`.
