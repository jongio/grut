#!/bin/sh
# Installer for grüt — a terminal file explorer with Git integration.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh -s -- v0.1.0
#   VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh
#
# Arguments:
#   [version]    Version to install (e.g. v0.1.0, 0.1.0). Defaults to latest.
#
# Environment variables:
#   VERSION      Override the version to install (e.g. v0.1.0). Defaults to latest.
#                A positional argument takes precedence over this variable.
#   INSTALL_DIR  Override the installation directory. Defaults to /usr/local/bin or ~/.local/bin.

set -eu

# ---------------------------------------------------------------------------
# Argument parsing — positional version overrides $VERSION env var
# ---------------------------------------------------------------------------
if [ $# -ge 1 ] && [ -n "$1" ]; then
    VERSION="$1"
fi

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
REPO="jongio/grut"
BINARY="grut"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"
GITHUB_DOWNLOAD="https://github.com/${REPO}/releases/download"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
info() { printf '  \033[1;34m→\033[0m %s\n' "$*"; }
pass() { printf '  \033[1;32m✓\033[0m %s\n' "$*"; }
warn() { printf '  \033[1;33m⚠\033[0m %s\n' "$*" >&2; }
fail() { printf '  \033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# HTTP helpers — prefer curl, fall back to wget
# ---------------------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
    http_get()      { curl -fsSL "$1"; }
    http_download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    http_get()      { wget -qO- "$1"; }
    http_download() { wget -qO "$2" "$1"; }
else
    fail "Either curl or wget is required."
fi

# ---------------------------------------------------------------------------
# SHA-256 helper — prefer sha256sum (Linux), fall back to shasum (macOS)
# ---------------------------------------------------------------------------
compute_sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        fail "Either sha256sum or shasum is required for checksum verification."
    fi
}

# ---------------------------------------------------------------------------
# Platform detection
# ---------------------------------------------------------------------------
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux"  ;;
        Darwin*) echo "darwin" ;;
        *)       fail "Unsupported operating system: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)        echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             fail "Unsupported architecture: $(uname -m)" ;;
    esac
}

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------
resolve_version() {
    if [ -n "${VERSION:-}" ]; then
        # Normalise: ensure the tag starts with "v".
        case "${VERSION}" in
            v*) echo "${VERSION}" ;;
            *)  echo "v${VERSION}" ;;
        esac
        return
    fi

    info "Querying GitHub for latest release…"

    # Parse the tag_name from the JSON response without requiring jq.
    tag=$(http_get "${GITHUB_API}" \
        | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
        | head -1 \
        | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//;s/"$//')

    if [ -z "${tag}" ]; then
        fail "Could not determine the latest version. Set VERSION and retry."
    fi

    echo "${tag}"
}

# ---------------------------------------------------------------------------
# Install-directory resolution
# ---------------------------------------------------------------------------
resolve_install_dir() {
    # Honour explicit override.
    if [ -n "${INSTALL_DIR:-}" ]; then
        echo "${INSTALL_DIR}"
        return
    fi

    # Root → system-wide path.
    if [ "$(id -u)" -eq 0 ]; then
        echo "/usr/local/bin"
        return
    fi

    # Non-root with sudo → system-wide path (will invoke sudo later).
    if command -v sudo >/dev/null 2>&1; then
        echo "/usr/local/bin"
        return
    fi

    # Fallback → user-local path.
    echo "${HOME}/.local/bin"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    printf '\n  \033[1mgrüt Installer\033[0m\n\n'

    # Ensure tar is available.
    command -v tar >/dev/null 2>&1 || fail "tar is required."

    # ---- Platform --------------------------------------------------------
    os="$(detect_os)"
    arch="$(detect_arch)"

    # macOS — use the actual architecture.
    if [ "${os}" = "darwin" ]; then
        archive_arch="${arch}"
        info "Detected platform: macOS/${arch}"
    else
        archive_arch="${arch}"
        info "Detected platform: ${os}/${arch}"
    fi

    # ---- Version ---------------------------------------------------------
    tag="$(resolve_version)"
    version="${tag#v}"                       # strip leading "v" for filename
    pass "Version: ${tag}"

    # ---- Build URLs ------------------------------------------------------
    archive_name="${BINARY}_${version}_${os}_${archive_arch}.tar.gz"
    checksums_name="${BINARY}_checksums.txt"
    archive_url="${GITHUB_DOWNLOAD}/${tag}/${archive_name}"
    checksums_url="${GITHUB_DOWNLOAD}/${tag}/${checksums_name}"

    # ---- Temp directory with cleanup trap --------------------------------
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT INT TERM

    # ---- Download --------------------------------------------------------
    info "Downloading ${archive_name}…"
    http_download "${archive_url}"  "${tmp}/${archive_name}"
    pass "Downloaded archive"

    # ---- Verify checksum -------------------------------------------------
    info "Verifying SHA-256 checksum…"
    http_download "${checksums_url}" "${tmp}/${checksums_name}"

    expected_sha=$(grep " ${archive_name}\$" "${tmp}/${checksums_name}" | awk '{print $1}')
    if [ -z "${expected_sha}" ]; then
        # Some checksum files use two-space separator; try a looser match.
        expected_sha=$(grep "${archive_name}" "${tmp}/${checksums_name}" | awk '{print $1}')
    fi
    if [ -z "${expected_sha}" ]; then
        fail "Archive '${archive_name}' not found in ${checksums_name}."
    fi

    actual_sha=$(compute_sha256 "${tmp}/${archive_name}")
    if [ "${actual_sha}" != "${expected_sha}" ]; then
        fail "Checksum mismatch!\n  expected: ${expected_sha}\n  got:      ${actual_sha}"
    fi
    pass "Checksum verified"

    # ---- Extract ---------------------------------------------------------
    info "Extracting…"
    tar xzf "${tmp}/${archive_name}" -C "${tmp}"

    # Locate the binary — handle both flat and nested archive layouts.
    if [ -f "${tmp}/${BINARY}" ]; then
        extracted_binary="${tmp}/${BINARY}"
    else
        extracted_binary="$(find "${tmp}" -name "${BINARY}" -type f | head -1)"
        if [ -z "${extracted_binary}" ]; then
            fail "${BINARY} not found in the downloaded archive."
        fi
    fi
    pass "Extracted ${BINARY}"

    # ---- Install ---------------------------------------------------------
    install_dir="$(resolve_install_dir)"
    install_path="${install_dir}/${BINARY}"
    needs_sudo=false

    if [ "${install_dir}" = "/usr/local/bin" ] && [ "$(id -u)" -ne 0 ]; then
        needs_sudo=true
    fi

    # Ensure target directory exists.
    if [ ! -d "${install_dir}" ]; then
        if [ "${needs_sudo}" = true ]; then
            sudo mkdir -p "${install_dir}"
        else
            mkdir -p "${install_dir}"
        fi
    fi

    if [ "${needs_sudo}" = true ]; then
        info "Installing to ${install_path} (via sudo)…"
        sudo install -m 755 "${extracted_binary}" "${install_path}"
    else
        info "Installing to ${install_path}…"
        install -m 755 "${extracted_binary}" "${install_path}"
    fi
    pass "Installed ${install_path}"

    # ---- PATH advisory ---------------------------------------------------
    if [ "${install_dir}" = "${HOME}/.local/bin" ]; then
        case ":${PATH}:" in
            *":${install_dir}:"*) ;;   # already in PATH
            *)
                echo ""
                warn "${install_dir} is not in your PATH."
                printf '     Add it by appending this line to your shell profile:\n'
                printf '\n       export PATH="%s:$PATH"\n\n' "${install_dir}"
                ;;
        esac
    fi

    # ---- Verify installation ---------------------------------------------
    if command -v "${BINARY}" >/dev/null 2>&1; then
        installed_version="$("${BINARY}" --version 2>/dev/null || true)"
        if [ -n "${installed_version}" ]; then
            pass "Verified: ${installed_version}"
        fi
    fi

    printf '\n  \033[1;32m✓ grüt %s installed successfully!\033[0m\n\n' "${tag}"
}

main
