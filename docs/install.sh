#!/usr/bin/env bash

set -euo pipefail

ARCHIVE_URL="https://openwallpaper.org/openwallpaper-linux-amd64.tar.xz"
CHECKSUM_URL="https://openwallpaper.org/openwallpaper-linux-amd64.tar.xz.sha256"
tmp_dir=""

cleanup() {
    if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
        rm -rf -- "$tmp_dir"
    fi
}
trap cleanup EXIT

run_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    else
        if ! command -v sudo >/dev/null 2>&1; then
            echo "error: sudo is required" >&2
            exit 1
        fi
        sudo "$@"
    fi
}

download() {
    local url="$1"
    local output="$2"
    local label="$3"
    local total pid size percent status status_file

    status_file="$output.status"
    total="$(curl -fsIL "$url" | awk 'tolower($1) == "content-length:" { value = $2 } END { gsub("\r", "", value); print value }')" || true
    case "$total" in
    "" | *[!0-9]* | 0) total=0 ;;
    esac

    (
        set +e
        curl -fsSL "$url" -o "$output"
        echo "$?" >"$status_file"
    ) &
    pid="$!"

    printf "%s 0%%" "$label" >&2
    while [ ! -s "$status_file" ]; do
        if [ "$total" -gt 0 ] && [ -e "$output" ]; then
            size="$(stat -c %s "$output" 2>/dev/null || echo 0)"
            percent=$((size * 100 / total))
            [ "$percent" -gt 99 ] && percent=99
            printf "\r%s %d%%" "$label" "$percent" >&2
        fi
        sleep 0.05
    done

    wait "$pid" 2>/dev/null || true
    status="$(cat "$status_file")"
    rm -f "$status_file"
    if [ "$status" -ne 0 ]; then
        echo >&2
        exit 1
    fi
    printf "\r%s 100%%\n" "$label" >&2
}

prompt_read() {
    local prompt="$1"

    if [ ! -r /dev/tty ]; then
        echo "error: unable to read input" >&2
        exit 1
    fi

    printf "%s" "$prompt" >/dev/tty
    if ! read -r REPLY </dev/tty; then
        echo "error: unable to read input" >&2
        exit 1
    fi
}

has_library() {
    local name="$1"

    if command -v ldconfig >/dev/null 2>&1 && ldconfig -p 2>/dev/null | grep -F -q "$name"; then
        return 0
    fi

    local path
    for path in /lib*/"$name"* /usr/lib*/"$name"* /usr/local/lib*/"$name"* /usr/lib/*/"$name"*; do
        [ -e "$path" ] && return 0
    done

    return 1
}

ask_install() {
    local message="$1"
    local package="$2"
    local answer

    while true; do
        prompt_read "$message install it? (y/n): "
        answer="$REPLY"
        case "$answer" in
        y | Y)
            packages+=("$package")
            return
            ;;
        n | N)
            return
            ;;
        *)
            echo "error: invalid input" >&2
            ;;
        esac
    done
}

ask_install_required() {
    local message="$1"
    local package="$2"
    local answer

    while true; do
        prompt_read "$message install it? (y/n): "
        answer="$REPLY"
        case "$answer" in
        y | Y)
            packages+=("$package")
            return
            ;;
        n | N)
            echo "error: xz not installed" >&2
            exit 1
            ;;
        *)
            echo "error: invalid input" >&2
            ;;
        esac
    done
}

select_install_type() {
    local answer

    while true; do
        {
            echo "select type of installation"
            echo "1) system-wide installation (/usr/local)"
            echo "2) user-wide installation (~/.local)"
        } >/dev/tty
        prompt_read "pick 1 or 2: "
        answer="$REPLY"
        case "$answer" in
        1 | 2)
            install_type="$answer"
            return
            ;;
        *)
            echo "error: invalid input" >&2
            ;;
        esac
    done
}

warn_missing() {
    echo "warning: $1 is not installed, it is required for $2, install it manually" >&2
}

case "$(uname -s)" in
Linux) ;;
*)
    echo "error: unsupported OS" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
x86_64 | amd64) ;;
*)
    echo "error: unsupported architecture" >&2
    exit 1
    ;;
esac

if ! command -v curl >/dev/null 2>&1; then
    echo "error: curl is required" >&2
    exit 1
fi

if ! command -v sha256sum >/dev/null 2>&1; then
    echo "error: sha256sum is not installed" >&2
    exit 1
fi

pm=""
if command -v apt-get >/dev/null 2>&1; then
    pm="apt"
elif command -v pacman >/dev/null 2>&1; then
    pm="pacman"
elif command -v dnf >/dev/null 2>&1; then
    pm="dnf"
fi

glslc_package=""
libmpv_package=""
libadwaita_package=""
libgirepository_package=""
xz_package=""
case "$pm" in
apt)
    glslc_package="glslc"
    libmpv_package="libmpv2"
    libadwaita_package="libadwaita-1-0"
    libgirepository_package="libgirepository-1.0-1"
    xz_package="xz-utils"
    ;;
pacman)
    glslc_package="shaderc"
    libmpv_package="mpv"
    libadwaita_package="libadwaita"
    libgirepository_package="gobject-introspection-runtime"
    xz_package="xz"
    ;;
dnf)
    glslc_package="glslc"
    libmpv_package="mpv-libs"
    libadwaita_package="libadwaita"
    libgirepository_package="gobject-introspection"
    xz_package="xz"
    ;;
esac

packages=()

if ! command -v xz >/dev/null 2>&1; then
    if [ -n "$pm" ]; then
        ask_install_required "xz is not installed, it is required to unpack the release archive," "$xz_package"
    else
        echo "error: xz not installed" >&2
        exit 1
    fi
fi

if ! command -v glslc >/dev/null 2>&1; then
    if [ -n "$pm" ]; then
        ask_install "glslc is not installed, it is required for wpe-compile (Wallpaper Engine support)," "$glslc_package"
    else
        warn_missing "glslc" "wpe-compile (Wallpaper Engine support)"
    fi
fi

if ! has_library "libmpv.so"; then
    if [ -n "$pm" ]; then
        ask_install "libmpv is not installed, it is required for video wallpaper support," "$libmpv_package"
    else
        warn_missing "libmpv" "video wallpaper support"
    fi
fi

if ! has_library "libadwaita-1.so"; then
    if [ -n "$pm" ]; then
        ask_install "libadwaita is not installed, it is required for UI," "$libadwaita_package"
    else
        warn_missing "libadwaita" "UI"
    fi
fi

if ! has_library "libgirepository-1.0.so"; then
    if [ -n "$pm" ]; then
        ask_install "libgirepository is not installed, it is required for UI," "$libgirepository_package"
    else
        warn_missing "libgirepository" "UI"
    fi
fi

if [ "${#packages[@]}" -gt 0 ]; then
    case "$pm" in
    apt) run_root apt-get install -y "${packages[@]}" ;;
    pacman) run_root pacman -S --needed --noconfirm "${packages[@]}" ;;
    dnf) run_root dnf install -y "${packages[@]}" ;;
    esac
fi

if [ "$(id -u)" -eq 0 ]; then
    install_type=1
else
    select_install_type
fi

tmp_dir="$(mktemp -d /tmp/openwallpaper-install.XXXXXX)"
ARCHIVE_PATH="$tmp_dir/openwallpaper-linux-amd64.tar.xz"
CHECKSUM_PATH="$tmp_dir/openwallpaper-linux-amd64.tar.xz.sha256"

download "$ARCHIVE_URL" "$ARCHIVE_PATH" "downloading release..."
download "$CHECKSUM_URL" "$CHECKSUM_PATH" "downloading checksum..."

if ! (cd "$tmp_dir" && sha256sum -c openwallpaper-linux-amd64.tar.xz.sha256 >/dev/null); then
    echo "error: checksums do not match" >&2
    exit 1
fi

tar -xf "$ARCHIVE_PATH" -C "$tmp_dir"
src="$tmp_dir/openwallpaper-linux-amd64"

case "$install_type" in
1)
    run_root install -Dm755 "$src/bin/wallpaperd" /usr/local/bin/wallpaperd
    run_root install -Dm755 "$src/bin/wallpaperd-wamrc" /usr/local/bin/wallpaperd-wamrc
    run_root install -Dm755 "$src/bin/wpe-compile" /usr/local/bin/wpe-compile
    run_root install -Dm755 "$src/bin/owui" /usr/local/bin/owui
    run_root install -Dm644 "$src/share/applications/owui.desktop" /usr/local/share/applications/owui.desktop
    ;;
2)
    install -Dm755 "$src/bin/wallpaperd" "$HOME/.local/bin/wallpaperd"
    install -Dm755 "$src/bin/wallpaperd-wamrc" "$HOME/.local/bin/wallpaperd-wamrc"
    install -Dm755 "$src/bin/wpe-compile" "$HOME/.local/bin/wpe-compile"
    install -Dm755 "$src/bin/owui" "$HOME/.local/bin/owui"
    install -Dm644 "$src/share/applications/owui.desktop" "$HOME/.local/share/applications/owui.desktop"
    ;;
*)
    echo "error: invalid installation type" >&2
    exit 1
    ;;
esac

echo "installation finished"
