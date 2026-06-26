#!/bin/sh
set -e

GITHUB_REPO="psviderski/uncloud"
INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
# Use the latest version or specify the version to install (the 'v' prefix is optional):
#   curl ... | VERSION=v1.2.3 sh
VERSION=${VERSION:-latest}

# Normalise numeric versions to a 'vX.Y.Z' release tag so VERSION can be passed with or without
# the 'v' prefix. Non-numeric refs such as 'nightly' are tags as-is and left untouched.
case "${VERSION#v}" in
    [0-9]*) VERSION="v${VERSION#v}" ;;
esac


# The CLI archive and binary were renamed from 'uncloud' to 'uc' in v0.20.0. Returns success (0) when VERSION
# is a numeric release older than v0.20.0, which still uses the legacy 'uncloud_*' archive and 'uncloud' binary name.
#  Newer versions and non-numeric refs such as 'nightly' use 'uc'.
is_legacy_version() {
    v="${VERSION#v}"
    major="${v%%.*}"
    rest="${v#*.}"
    minor="${rest%%.*}"
    case "$major" in ''|*[!0-9]*) return 1 ;; esac
    case "$minor" in ''|*[!0-9]*) return 1 ;; esac
    [ "$major" -eq 0 ] && [ "$minor" -lt 20 ]
}

print_manual_install() {
    RELEASES_URL="https://github.com/${GITHUB_REPO}/releases/${VERSION}"
    echo "Failed while attempting to install uncloud CLI. You can install it manually:"
    echo "  1. Open your web browser and go to ${RELEASES_URL}"
    echo "  2. Download uc_<OS>_<ARCH>.tar.gz for your platform (OS: linux/macos, ARCH: amd64/arm64)."
    echo "  3. Extract the 'uc' binary from the archive: tar -xvf uc_*.tar.gz"
    echo "  4. Install the binary to /usr/local/bin: sudo install ./uc ${INSTALL_DIR}/uc"
    echo "  5. Delete the downloaded archive and extracted binary: rm uc*"
    echo "  6. Run 'uc --help' to verify the installation. Enjoy! ✨"
}

fetch_latest_version() {
    latest_url="https://github.com/${GITHUB_REPO}/releases/latest"
    VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$latest_url" | grep -o 'tag/[^/]*$' | cut -d'/' -f2)
    if [ -z "$VERSION" ]; then
        echo "Failed to fetch the latest version from GitHub."
        print_manual_install
        exit 1
    fi
}

# Check if not running as root and need to use sudo to write to INSTALL_DIR.
SUDO=""
if [ "$(id -u)" != "0" ] && [ ! -w "$INSTALL_DIR" ]; then
    if ! command -v sudo >/dev/null 2>&1; then
        echo "Please run this script as root or install sudo."
        print_manual_install
        exit 1
    fi
    SUDO="sudo"
fi

# Detect the user OS and architecture.
OS=$(uname -s)
ARCH=$(uname -m)
case "$OS" in
    Darwin) BINARY_OS="macos" ;;
    Linux)  BINARY_OS="linux" ;;
    *)
        echo "There is no uncloud CLI support for $OS/$ARCH. Please open a GitHub issue if you would like to request support."
        exit 1
        ;;
esac
case "$ARCH" in
    aarch64 | arm64) BINARY_ARCH="arm64" ;;
    x86_64)          BINARY_ARCH="amd64" ;;
    *)
        echo "There is no uncloud CLI support for $OS/$ARCH. Please open a GitHub issue if you would like to request support."
        exit 1
        ;;
esac

# Use the latest version if not specified explicitly.
if [ "$VERSION" = "latest" ]; then
    fetch_latest_version
fi

# Pick the archive and binary name for the requested version. Pre-v0.20.0 releases ship the legacy 'uncloud' name.
# v0.20.0 and newer ship 'uc'.
CLI_NAME="uc"
if is_legacy_version; then
    CLI_NAME="uncloud"
fi
BINARY_NAME="${CLI_NAME}_${BINARY_OS}_${BINARY_ARCH}.tar.gz"
BINARY_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"
CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/$VERSION/checksums.txt"

# Create a temporary directory for downloads.
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download the binary and checksums file.
echo "Downloading uc binary ${VERSION} ${BINARY_URL}"
curl -fsSL "$BINARY_URL" -o "${TMP_DIR}/${BINARY_NAME}"
curl -fsSL "$CHECKSUM_URL" -o "${TMP_DIR}/checksums.txt"
echo "Download complete."

echo "Verifying checksum..."
cd "$TMP_DIR"
if ! sha256sum --check --ignore-missing "checksums.txt"; then
    echo "Checksum verification failed."
    print_manual_install
    exit 1
fi
echo "Checksum is valid."

# Decompress and install the binary.
tar -xf "${BINARY_NAME}"

if [ -z "${SUDO}" ]; then
    echo "Installing uc binary to ${INSTALL_DIR}"
else
    echo "Installing uc binary to ${INSTALL_DIR} using sudo. You may be prompted for your password."
fi
if ! $SUDO install "./${CLI_NAME}" "${INSTALL_DIR}/uc"; then
    echo "Failed to install uc binary to ${INSTALL_DIR}"
    print_manual_install
    exit 1
fi

echo "Successfully installed uc binary ${VERSION} to ${INSTALL_DIR}/uc ✨"
