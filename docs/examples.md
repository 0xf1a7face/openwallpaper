\page examples Examples

Repository contains some small scenes that demonstrate OpenWallpaper API usage.

## Building examples

To build examples yourself, you will need to install:

- WASM C compiler, [wasi-sdk](https://github.com/WebAssembly/wasi-sdk/releases) recommended
- glslc

Clone the repository:

```sh
git clone --depth=1 --recurse-submodules https://github.com/mechakotik/openwallpaper
cd openwallpaper
```

Build examples, specifying the path to WASM C compiler in `WASM_CC` environment variable:

```sh
WASM_CC=/opt/wasi-sdk/bin/clang ./examples/build.sh
```

Each script writes `scene.wasm` and generated assets to a `scene` directory in the corresponding example directory. Run an example with:

```sh
wallpaperd examples/triangle/scene/scene.wasm
```

## triangle

Draws a triangle in the center of the screen, classic example of graphics API usage. You can see an explanation of how this code works in [Drawing a triangle](\ref triangle) developer guide section.

<div style="text-align:left;">
    <img src="triangle.webp" width="600">
</div>

[Source code](https://github.com/mechakotik/openwallpaper/tree/main/examples/triangle)

## fullscreen-shader

Demonstrates how to run shader wallpapers (e.g. from ShaderToy) in OpenWallpaper. Shader that was ported is [Singularity by @XorDev](https://www.shadertoy.com/view/3csSWB).

<div style="text-align:left;">
    <img src="fullscreen-shader.webp" width="600">
</div>

[Source code](https://github.com/mechakotik/openwallpaper/tree/main/examples/fullscreen-shader)

## image

Draws a static image, simplest example of texture rendering. Original image is Scarlett Tree from KDE Plasma 6 artwork.

<div style="text-align:left;">
    <img src="image.webp" width="600">
</div>

[Source code](https://github.com/mechakotik/openwallpaper/tree/main/examples/image)

## rain

Draws particles to create a rain-like effect. An example of using instanced rendering to draw a lot of similar objects efficiently.

<div style="text-align:left;">
    <img src="rain.webp" width="600">
</div>

[Source code](https://github.com/mechakotik/openwallpaper/tree/main/examples/rain)

## visualizer

Example of using `ow_get_audio_spectrum` to visualize audio. It gets spectrum data and draws it as white bars on the screen, without any preprocessing.

<div style="text-align:left;">
    <img src="visualizer.webp" width="600">
</div>

[Source code](https://github.com/mechakotik/openwallpaper/tree/main/examples/visualizer)

