#!/bin/sh
set -e

GITHUB_REPO="psviderski/uncloud"
INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
# Use the latest version or specify the version to install:
#   curl ... | VERSION=v1.2.3 sh
VERSION=${VERSION:-latest}

print_manual_install() {
    # TODO: review
    echo "You can manually install uncloud by:"
    echo "1. Opening $RELEASES_URL"
    echo "2. Downloading uncloud_*_${OS}_${ARCH}.tar.gz for your platform"
    echo "3. Verifying the checksum from checksums.txt"
    echo "4. Extracting the archive: tar xzf uncloud_*_${OS}_${ARCH}.tar.gz"
    echo "5. Installing the binary: sudo install -m 755 uncloud /usr/local/bin/"
    echo "6. Creating symlink: sudo ln -sf /usr/local/bin/uncloud /usr/local/bin/uc"
}

latest_version() {
    api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    version=$(curl -fsSL "$api_url" | grep -o '"tag_name": "[^"]*' | cut -d'"' -f4)
    if [ -z "$version" ]; then
        echo "Failed to fetch the latest version from GitHub."
        print_manual_install
        exit 1
    fi
    echo "$version"
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
    VERSION=$(latest_version)
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

# TODO: fix name_template in goreleaser config.
#echo "Verifying checksum..."
cd "$TMP_DIR"
#if ! sha256sum --check --ignore-missing "checksums.txt"; then
#    echo "Checksum verification failed."
#    print_manual_install
#    exit 1
#fi
#echo "Checksum is valid."

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
