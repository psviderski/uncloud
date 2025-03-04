#!/usr/bin/env bash

set -euo pipefail

AUTO_CONFIRM=${AUTO_CONFIRM:-false}
# Define variables based on the same defaults used in the installation script.
INSTALL_BIN_DIR=${INSTALL_BIN_DIR:-/usr/local/bin}
INSTALL_SYSTEMD_DIR=${INSTALL_SYSTEMD_DIR:-/etc/systemd/system}
UNCLOUD_USER="uncloud"
UNCLOUD_DATA_DIR=${UNCLOUD_DATA_DIR:-/var/lib/uncloud}
UNCLOUD_RUN_DIR=${UNCLOUD_RUN_DIR:-/run/uncloud}

log() {
    echo -e "\033[1;32m$1\033[0m"
}

error() {
    echo -e "\033[1;31mERROR: $1\033[0m" >&2
    exit 1
}

confirm() {
    if [ "${AUTO_CONFIRM}" = "true" ]; then
        return 0
    fi

    read -r -p "$1 [y/N] " response
    case "$response" in
        [yY][eE][sS]|[yY])
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Check if user is root.
if [ "$EUID" -ne 0 ]; then
    error "Please run the uninstall script with sudo or as root."
fi

# Confirm before proceeding.
log "⚠️This script will uninstall Uncloud and remove ALL Uncloud managed containers on this machine."
log "The following actions will be performed:"
echo "- Remove Uncloud systemd services"
echo "- Remove Uncloud binaries and data"
echo "- Remove Uncloud user and group"
echo "- Remove all Docker containers managed by Uncloud"
echo "- Remove Uncloud Docker network"
echo "- Remove Uncloud WireGuard interface"

if ! confirm "Do you want to proceed with uninstallation?"; then
    log "Uninstallation cancelled."
    exit 0
fi

log "⏳ Stopping systemd services..."
systemctl stop uncloud.service || log "uncloud.service not running or doesn't exist."
systemctl stop uncloud-corrosion.service || log "uncloud-corrosion.service not running or doesn't exist."
systemctl disable uncloud.service || log "uncloud.service already disabled or doesn't exist."
systemctl disable uncloud-corrosion.service || log "uncloud-corrosion.service already disabled or doesn't exist."
log "✓ Systemd services stopped."

log "⏳ Removing systemd service files..."
rm -fv "${INSTALL_SYSTEMD_DIR}/uncloud.service"
rm -fv "${INSTALL_SYSTEMD_DIR}/uncloud-corrosion.service"
systemctl daemon-reload
log "✓ Systemd service files removed."

log "⏳ Removing binaries..."
rm -fv "${INSTALL_BIN_DIR}/uncloudd"
rm -fv "${INSTALL_BIN_DIR}/uncloud-corrosion"
log "✓ Binaries removed."

log "⏳ Removing data and run directories..."
rm -rfv "${UNCLOUD_DATA_DIR}"
rm -rfv "${UNCLOUD_RUN_DIR}"
log "✓ Data and run directories removed."

log "⏳ Removing Linux user and group..."
if id "${UNCLOUD_USER}" &> /dev/null; then
    userdel "${UNCLOUD_USER}" || error "Failed to remove user ${UNCLOUD_USER}, it may still be used by other processes."
    log "✓ Linux user '${UNCLOUD_USER}' removed."
else
    log "Linux user '${UNCLOUD_USER}' does not exist."
fi

# Check if the group still exists (other users may be using it).
if getent group "${UNCLOUD_USER}" &> /dev/null; then
    groupdel "${UNCLOUD_USER}" || error "Failed to remove group ${UNCLOUD_USER}, it may still be used by other processes or users."
    log "✓ Linux group '${UNCLOUD_USER}' removed."
else
    log "Linux group '${UNCLOUD_USER}' does not exist or was already removed."
fi

log "⏳ Looking for Docker containers and network created by Uncloud..."
if command -v docker &> /dev/null; then
    uncloud_containers=$(docker ps -a --filter "label=uncloud.managed" -q)
    if [ -n "${uncloud_containers}" ]; then
        log "Found $(echo "${uncloud_containers}" | wc -w) Uncloud managed containers."
        log "⏳ Stopping Uncloud managed containers..."
        echo "${uncloud_containers}" | xargs docker stop || error "Failed to stop Uncloud managed containers."
        log "⏳ Removing Uncloud managed containers..."
        echo "${uncloud_containers}" | xargs docker rm || error "Failed to remove Uncloud managed containers."
        log "✓ Uncloud managed containers stopped and removed."
    else
        log "No Uncloud managed containers found."
    fi

    uncloud_network=$(docker network ls --filter "name=uncloud" -q)
    if [ -n "${uncloud_network}" ]; then
        log "⏳ Removing Docker network uncloud..."
        docker network rm uncloud || error "Failed to remove Docker network uncloud."
        log "✓ Docker network uncloud removed."
    else
        log "Docker network uncloud not found."
    fi
else
    log "Docker CLI not found, skipping Docker container and network cleanup."
fi

log "⏳ Removing WireGuard interface uncloud..."
if ip link show uncloud &> /dev/null; then
    ip link delete uncloud
    log "✓ WireGuard interface uncloud removed."
else
    log "WireGuard interface uncloud not found."
fi

log "⏳ Removing uninstall script..."
rm -fv "${INSTALL_BIN_DIR}/uncloud-uninstall"
log "✓ Uninstall script removed."

echo
log "✅ Uncloud has been uninstalled successfully!"
log "Note: Docker installation was preserved. If you want to completely remove Docker as well, \
follow https://docs.docker.com/engine/install/ubuntu/#uninstall-docker-engine"
