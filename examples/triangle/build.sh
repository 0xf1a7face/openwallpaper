#!/bin/sh

set -e
cd "$(dirname "$0")" || exit 1

if ! [ -n "$WASM_CC" ]; then
    echo "error: WASM_CC is not set"
    exit 1
fi

if ! command -v glslc >/dev/null 2>&1; then
    echo "error: glslc not found"
    exit 1
fi

rm -rf scene
mkdir -p scene

$WASM_CC scene.c \
    -o scene/scene.wasm \
    -I../../include \
    -mexec-model=reactor \
    -Wl,--allow-undefined \
    -O3 \
    -flto

glslc -fshader-stage=vertex vertex.glsl -o scene/vertex.spv
glslc -fshader-stage=fragment fragment.glsl -o scene/fragment.spv
