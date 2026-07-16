\page install Installation

This guide shows different ways to install OpenWallpaper.

## Automatic binary installation

This is the easiest way to install OpenWallpaper. It will check for dependencies, automatically install them (if you are using apt, pacman, or dnf), download latest binary release, and install it system-wide (`/usr/local`) or user-wide (`~/.local`), to your preference.

```sh
curl -fsSL https://openwallpaper.org/install.sh | bash
```

Note that binary release is available only for x86_64 Linux with glibc 2.41 (Debian 13) or newer. If your setup doesn't meet these requirements, you may try building from source (described below).

## Manual binary installation

First, you will need to install dependencies in your distro specific way:

- libmpv *(optional, for video wallpapers)*
- glslc *(optional, for Wallpaper Engine support)*
- libadwaita, libgirepository *(optional, for UI)*

Download latest binary release and verify checksum:

```sh
curl -fLO https://openwallpaper.org/openwallpaper-linux-amd64.tar.xz
curl -fLO https://openwallpaper.org/openwallpaper-linux-amd64.tar.xz.sha256
sha256sum -c openwallpaper-linux-amd64.tar.xz.sha256
```

Extract release archive:

```sh
tar -xf openwallpaper-linux-amd64.tar.xz
cd openwallpaper-linux-amd64
```

Install system-wide:

```sh
sudo install -Dm755 bin/wallpaperd /usr/local/bin/wallpaperd
sudo install -Dm755 bin/wallpaperd-wamrc /usr/local/bin/wallpaperd-wamrc
sudo install -Dm755 bin/wpe-compile /usr/local/bin/wpe-compile
sudo install -Dm755 bin/owui /usr/local/bin/owui
sudo install -Dm644 share/applications/owui.desktop /usr/local/share/applications/owui.desktop
```

If you want user-wide installation, replace `/usr/local` with `~/.local` (ensure it's in your `$PATH`), and run without sudo.

## Building from source

First, you will need to dependencies. For wallpaperd:

- C/C++ compiler
- CMake
- LLVM *(optional, for scene AOT compilation)*
- libmpv (*optional, for video and GIF wallpapers*)
- wayland-scanner *(optional, for wlr-layer-shell output support)*
- libpipewire, libspa *(optional, for PipeWire audio visualizer)*
- libpulse-simple *(optional, for PulseAudio audio visualizer)*
- PortAudio *(optional, for PortAudio audio visualizer)*

For wpe-compile (Wallpaper Engine support):

- Go compiler
- glslc
- WASM C compiler with wasi support, e.g. wasi-sdk or clang + wasi-libc

For owui:

- Go compiler
- libadwaita
- libgirepository

After you have installed dependencies, clone OpenWallpaper sources:

```sh
git clone --depth=1 --recurse-submodules https://github.com/mechakotik/openwallpaper
cd openwallpaper
```

Build and install wallpaperd:

```sh
cd wallpaperd
mkdir build && cd build
cmake ..
cmake --build . -j$(nproc)
sudo cmake --install .
cd ..
```

Build and install wpe-compile (replace `WASM_CC` value with your WASM C compiler command if needed):

```sh
cd wpe-compile
WASM_CC="clang --target=wasm32-wasi --sysroot=/usr" ./module/build.sh
go build .
sudo install -Dm755 wpe-compile /usr/local/bin/wpe-compile
cd ..
```

Build and install owui:

```sh
cd owui
go build .
sudo install -Dm755 owui /usr/local/bin/owui
sudo install -Dm644 owui.desktop /usr/local/share/applications/owui.desktop
cd ..
```

## Building binary release

If you want to make binary release archive like ones used in binary installation (`openwallpaper-linux-amd64.tar.xz`), you will need to install Docker. Then clone OpenWallpaper sources and run build script:

```sh
git clone --depth=1 --recurse-submodules https://github.com/mechakotik/openwallpaper
cd openwallpaper
./build_dist.sh
```

Resulting archive and checksum will be located in `dist` directory.

