#!/bin/sh

set -e
cd "$(dirname "$0")" || exit 1

if ! [ -n "$WASM_CC" ]; then
    echo "error: WASM_CC is not set"
    exit 1
fi

rm -f scene.wasm

$WASM_CC \
    common.c \
    image.c \
    main.c \
    particle.c \
    puppet.c \
    renderer.c \
    scene_bin.c \
    transform.c \
    uniform.c \
    -o scene.wasm \
    -mexec-model=reactor \
    -Wl,--allow-undefined \
    -O3 \
    -flto
