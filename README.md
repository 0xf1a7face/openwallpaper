<h1 align="center">
    openwallpaper
</h1>

<div align="center">
    <p>
        <a href="https://openwallpaper.org">Website</a> •
        <a href="https://openwallpaper.org/install.html">Installation</a> •
        <a href="https://openwallpaper.org/examples.html">Examples</a> •
        <a href="https://openwallpaper.org/openwallpaper_8h.html">API reference</a>
    </p>
</div>

OpenWallpaper is a programmable live wallpaper runtime inspired by Wallpaper Engine. It supports KDE Plasma Wayland, Hyprland and wlroots-based compositors through wlr-layer-shell protocol.

## Install

Automatically install binary release on most Linux distributions:

```sh
curl -fsSL https://openwallpaper.org/install.sh | bash
```

See [Installation](https://openwallpaper.org/install.html) for manual binary installation, as well as building from source.

## Features

- **OpenWallpaper scenes** - real-time live wallpapers similar to Wallpaper Engine scenes, but using WASM instead of limiting declarative format. If you can code it, you can make it a wallpaper, while having lightweight runtime and native performance with AOT compilation

- **Wallpaper Engine scenes** - can be converted to OpenWallpaper scenes with wpe-compile

- **Video and GIF wallpapers** - supported through libmpv

- **Auto pause** - try to detect when wallpaper is covered by another window and can't be visible, and pause rendering, saving CPU and GPU resources (DE-dependent, only Hyprland is fully supported for now)

- **Multiple monitors** - render to specific display when it's connected, and sleep without wasting resources when it's not

- **Audio visualization** - scene API exposes audio spectrum data through cava

- **Modular GUI** - made with GTK4 and libadwaita, is not required for renderer to work and doesn't hang in background

## Documentation

Check out official OpenWallpaper website at [openwallpaper.org](https://openwallpaper.org) for more information.

