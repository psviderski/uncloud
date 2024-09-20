#!/usr/bin/env bash

set -euo pipefail

INSTALL_BIN_DIR=${INSTALL_BIN_DIR:-/usr/local/bin}
INSTALL_SYSTEMD_DIR=${INSTALL_SYSTEMD_DIR:-/etc/systemd/system}
UNCLOUD_GITHUB_URL="https://github.com/psviderski/uncloud"
UNCLOUD_VERSION=${UNCLOUD_VERSION:-latest}
UNCLOUD_GROUP="uncloud"
UNCLOUD_GROUP_ADD_USER=${UNCLOUD_GROUP_ADD_USER:-}

log() {
    echo -e "\033[1;32m$1\033[0m" >&2
}

error() {
    echo -e "\033[1;31mERROR: $1\033[0m" >&2
    exit 1
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

verify_system() {
  if [[ "$(uname -s)" != "Linux" ]]; then
      error "Uncloud machine must be a Linux system. Your system ($(uname -s)) is not supported."
  fi

  local arch
  arch=$(uname -m)
  if [[ "$arch" != "x86_64" && "$arch" != "aarch64" ]]; then
      error "Uncloud machine must have amd64 (x86_64) or arm64 (aarch64) architecture. \
Your system architecture ($arch) is not supported."
  fi

  if [[ ! -d /run/systemd/system ]]; then
      error "Cannot find systemd to use as a service manager for the Uncloud machine daemon. \
Uncloud supports only systemd-based Linux systems for now."
  fi
}

install_docker() {
    if command_exists docker; then
        log "✓ Docker is already installed."
        docker version
        return
    fi

    log "⏳ Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    log "✓ Docker installed successfully."
}

create_uncloud_group() {
    if getent group "${UNCLOUD_GROUP}" > /dev/null; then
      log "✓ Linux group 'uncloud' already exists."
    else
      if ! groupadd --system "${UNCLOUD_GROUP}"; then
        error "Failed to create Linux group 'uncloud'."
      fi
      log "✓ Linux group 'uncloud' created."
    fi

    if [ -n "${UNCLOUD_GROUP_ADD_USER}" ]; then
        if ! gpasswd --add "${UNCLOUD_GROUP_ADD_USER}" "${UNCLOUD_GROUP}" > /dev/null; then
            error "Failed to add user '${UNCLOUD_GROUP_ADD_USER}' to group '${UNCLOUD_GROUP}'."
        fi
        log "✓ Linux user '${UNCLOUD_GROUP_ADD_USER}' added to group '${UNCLOUD_GROUP}'."
    fi
}

install_uncloud_binaries() {
    local arch
    local file_arch

    arch=$(uname -m)
    case $arch in
      x86_64)
        file_arch="amd64"
        ;;
      aarch64)
        file_arch="arm64"
        ;;
      *)
        error "Unsupported architecture: ${arch}"
        ;;
    esac

    local uncloudd_install_path="${INSTALL_BIN_DIR}/uncloudd"
    if [ -f "${uncloudd_install_path}" ]; then
        log "✓ uncloudd binary is already installed."
        return
    fi

    log "⏳ Installing Uncloud binaries..."

    # Create a temporary directory for downloads.
    local tmp_dir
    tmp_dir=$(mktemp -d)
    # Ensure the temporary directory is deleted on script exit.
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp_dir'" EXIT

    local uncloudd_url
    if [ "${UNCLOUD_VERSION}" == "latest" ]; then
        uncloudd_url="${UNCLOUD_GITHUB_URL}/releases/latest/download/uncloudd-${file_arch}"
    else
        uncloudd_url="${UNCLOUD_GITHUB_URL}/releases/download/${UNCLOUD_VERSION}/uncloudd-${file_arch}"
    fi
    local uncloudd_download_path="${tmp_dir}/uncloudd"

    if ! curl -fsSL -o "${uncloudd_download_path}" "${uncloudd_url}"; then
        error "Failed to download uncloudd binary: ${uncloudd_url}"
    fi
    if ! install "${uncloudd_download_path}" "${uncloudd_install_path}"; then
        error "Failed to install uncloud binary to ${uncloudd_install_path}"
    fi
    log "✓ uncloudd binary installed to ${uncloudd_install_path}"

    # TODO: install uncloud CLI binary and create a uc alias.
}

install_uncloud_systemd() {
    local uncloud_service_path="${INSTALL_SYSTEMD_DIR}/uncloud.service"
    if [ -f "${uncloud_service_path}" ]; then
      log "⏳ Updating systemd unit for Uncloud machine daemon..."
    else
      log "⏳ Installing systemd unit for Uncloud machine daemon..."
    fi

    cat << EOF | sudo tee "${INSTALL_SYSTEMD_DIR}/uncloud.service" > /dev/null
[Unit]
Description=Uncloud machine daemon
After=network-online.target
Wants=network-online.target
Requires=uncloud.socket

[Service]
Type=simple
ExecStart=/usr/local/bin/uncloudd
Restart=always
RestartSec=2

# Hardening options.
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true
ProtectHome=true
ProtectControlGroups=true
ProtectKernelTunables=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
RestrictNamespaces=true

[Install]
WantedBy=multi-user.target
EOF
    log "✓ Systemd unit file created: ${uncloud_service_path}"

    # Reload systemd to recognize the new or updated unit file.
    systemctl daemon-reload
    systemctl enable uncloud.service
    systemctl start uncloud.service
    log "✓ Uncloud machine daemon started."
}

log "⏳ Running Uncloud install script..."

if [ "$EUID" -ne 0 ]; then
    error "Please run the install script with sudo or as root."
fi

verify_system
install_docker
create_uncloud_group
install_uncloud_binaries
install_uncloud_systemd

log "✓ Uncloud installed on the machine successfully! 🎉"
