#!/bin/sh
set -e

GITHUB_REPO="psviderski/uncloud"
INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
# Use the latest version or specify the version to install:
#   curl ... | VERSION=v1.2.3 sh
VERSION=${VERSION:-latest}

print_manual_install() {
    RELEASES_URL="https://github.com/${GITHUB_REPO}/releases/${VERSION}"
    echo "Failed while attempting to install uncloud CLI. You can install it manually:"
    echo "  1. Open your web browser and go to ${RELEASES_URL}"
    echo "  2. Download uncloud_<OS>_<ARCH>.tar.gz for your platform (OS: linux/macos, ARCH: amd64/arm64)."
    echo "  3. Extract the 'uncloud' binary from the archive: tar -xvf uncloud_*.tar.gz"
    echo "  4. Install the binary to /usr/local/bin: sudo install ./uncloud ${INSTALL_DIR}/uncloud"
    echo "  5. Optionally create a 'uc' symlink: sudo ln -sf ${INSTALL_DIR}/uncloud ${INSTALL_DIR}/uc"
    echo "  6. Delete the downloaded archive and extracted binary: rm uncloud*"
    echo "  7. Run 'uncloud --help' to verify the installation. Enjoy! ✨"
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
BINARY_NAME="uncloud_${BINARY_OS}_${BINARY_ARCH}.tar.gz"
BINARY_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"
CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/$VERSION/checksums.txt"

# Create a temporary directory for downloads.
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download the binary and checksums file.
echo "Downloading uncloud binary ${VERSION} ${BINARY_URL}"
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
    echo "Installing uncloud binary to ${INSTALL_DIR}"
else
    echo "Installing uncloud binary to ${INSTALL_DIR} using sudo. You may be prompted for your password."
fi
if ! $SUDO install ./uncloud "${INSTALL_DIR}/uncloud"; then
    echo "Failed to install uncloud binary to ${INSTALL_DIR}"
    print_manual_install
    exit 1
fi
# Create 'uc' shortcut symlink.
$SUDO ln -sf "${INSTALL_DIR}/uncloud" "${INSTALL_DIR}/uc"

echo "Successfully installed uncloud binary ${VERSION} to ${INSTALL_DIR}/uncloud"
echo "Created a shortcut command 'uc' for convenience ✨"
