#!/usr/bin/env bash

set -euo pipefail

if [ "${1:-}" != "--inside" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        echo "error: run this script as your normal user, not as root" >&2
        echo "it will use sudo only for docker if needed" >&2
        exit 1
    fi

    root="$(cd "$(dirname "$0")" && pwd)"
    mkdir -p "$root/dist"

    if ! command -v docker >/dev/null 2>&1; then
        echo "error: install docker" >&2
        exit 1
    fi
    docker=(docker)
    if ! docker info >/dev/null 2>&1; then
        docker=(sudo docker)
    fi

    exec "${docker[@]}" run --rm \
        -e "HOST_UID=$(id -u)" \
        -e "HOST_GID=$(id -g)" \
        -v "$root:/src:ro" \
        -v "$root/dist:/out" \
        debian:13 \
        bash /src/build_dist.sh --inside
fi

apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    clang \
    curl \
    git \
    lld \
    build-essential \
    libadwaita-1-dev \
    libclang-rt-19-dev-wasm32 \
    libegl1-mesa-dev \
    libgl-dev \
    libgirepository1.0-dev \
    libgtk-4-dev \
    libmpv-dev \
    libpipewire-0.3-dev \
    libpulse-dev \
    libspa-0.2-dev \
    libxtst-dev \
    libvulkan-dev \
    libwayland-dev \
    libxkbcommon-dev \
    ninja-build \
    pkg-config \
    portaudio19-dev \
    python3 \
    python3-pip \
    python3-venv \
    tar \
    wasi-libc \
    xz-utils

python3 -m pip install --break-system-packages "cmake>=4.0,<5"
curl -fsSL "https://go.dev/dl/go1.26.2.linux-amd64.tar.gz" -o /tmp/go.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz

export PATH="/usr/local/go/bin:$PATH"
export CC=clang
export CXX=clang++

rm -rf /work/openwallpaper
mkdir -p /work/openwallpaper
tar \
    --exclude='./.git' \
    --exclude='./build' \
    --exclude='./dist' \
    -C /src -cf - . |
    tar -C /work/openwallpaper -xf -

cd /work/openwallpaper

cmake -S wallpaperd -B build/wallpaperd -G Ninja \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX=/usr \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++ \
    -DCMAKE_EXE_LINKER_FLAGS="-static-libstdc++ -static-libgcc" \
    -DWD_WAMRC_STATIC_LLVM=ON
cmake --build build/wallpaperd --parallel
DESTDIR="$PWD/stage/wallpaperd" cmake --install build/wallpaperd --strip

export WASM_CC="${WASM_CC:-clang --target=wasm32-wasi --sysroot=/usr}"
(cd wpe-compile/module && ./build.sh)
(cd wpe-compile && go build -trimpath -ldflags="-s -w" -o ../stage/wpe-compile .)
(cd owui && go build -trimpath -ldflags="-s -w" -o ../stage/owui .)

archive="openwallpaper-linux-amd64"
root="dist/$archive"

mkdir -p \
    "$root/bin" \
    "$root/share/applications"

install -m 0755 stage/wallpaperd/usr/bin/wallpaperd "$root/bin/wallpaperd"
install -m 0755 stage/wallpaperd/usr/bin/wallpaperd-wamrc "$root/bin/wallpaperd-wamrc"
install -m 0755 stage/wpe-compile "$root/bin/wpe-compile"
install -m 0755 stage/owui "$root/bin/owui"
install -m 0644 owui/owui.desktop "$root/share/applications/owui.desktop"

tar -cJf "/out/$archive.tar.xz" -C dist "$archive"
(cd /out && sha256sum "$archive.tar.xz" >"$archive.tar.xz.sha256")

chown "$HOST_UID:$HOST_GID" "/out/$archive.tar.xz" "/out/$archive.tar.xz.sha256"

echo "wrote dist/$archive.tar.xz"
