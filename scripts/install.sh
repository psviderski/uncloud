#!/usr/bin/env bash

set -euo pipefail

# If set to 'true', only install the packages and dependencies, without running, reloading, or
# restarting services or systemd.
INSTALL_ONLY=${INSTALL_ONLY:-false}

INSTALL_BIN_DIR=${INSTALL_BIN_DIR:-/usr/local/bin}
INSTALL_SYSTEMD_DIR=${INSTALL_SYSTEMD_DIR:-/etc/systemd/system}

UNCLOUD_GITHUB_URL="https://github.com/psviderski/uncloud"
UNCLOUD_VERSION=${UNCLOUD_VERSION:-latest}
# Remove the 'v' prefix from the version if it exists.
UNCLOUD_VERSION=${UNCLOUD_VERSION#v}
UNCLOUD_USER="uncloud"
# Add the specified Linux user to group $UNCLOUD_USER to allow the user to run uncloud commands without sudo.
UNCLOUD_GROUP_ADD_USER=${UNCLOUD_GROUP_ADD_USER:-}
UNCLOUD_DATA_DIR=${UNCLOUD_DATA_DIR:-/var/lib/uncloud}

CORROSION_GITHUB_URL="https://github.com/psviderski/corrosion"
CORROSION_VERSION=${CORROSION_VERSION:-v0.2.2}

DOCKER_ALREADY_INSTALLED=false
CONTAINERD_IMAGE_STORE_ENABLED=false
DOCKER_DAEMON_CONFIG_FILE=${DOCKER_DAEMON_CONFIG_FILE:-/etc/docker/daemon.json}
# Docker daemon configuration optimised for Uncloud.
DOCKER_DAEMON_CONFIG='{
  "features": {
    "containerd-snapshotter": true
  },
  "live-restore": true
}'

log() {
    echo -e "\033[1;32m$1\033[0m"
}

warning() {
    echo -e "\033[1;33m$1\033[0m"
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

  if [[ ! -d /run/systemd/system && "${INSTALL_ONLY}" != "true" ]]; then
      error "Cannot find systemd to use as a service manager for the Uncloud machine daemon. \
Uncloud supports only systemd-based Linux systems for now."
  fi
}

install_docker() {
    if command_exists dockerd; then
        log "✓ Docker is already installed."
        DOCKER_ALREADY_INSTALLED=true

        if [[ "${INSTALL_ONLY}" == "true" ]]; then
            return
        fi

        docker version

        # Check if the installed Docker configured to use the containerd image store.
        local driver_status
        driver_status=$(docker info -f '{{ .DriverStatus }}' 2>/dev/null)
        if [[ "$driver_status" == *"io.containerd.snapshotter"* ]]; then
            CONTAINERD_IMAGE_STORE_ENABLED="true"
        fi

        return
    fi

    log "⏳ Installing Docker..."
    curl -fsSL https://get.docker.com | sh

    # Configure Docker daemon for new installation.
    # Create Docker daemon config directory if it doesn't exist.
    local docker_config_dir
    docker_config_dir=$(dirname "${DOCKER_DAEMON_CONFIG_FILE}")
    if [ ! -d "${docker_config_dir}" ]; then
        mkdir -p "${docker_config_dir}"
    fi

    log "⏳ Configuring Docker daemon (${DOCKER_DAEMON_CONFIG_FILE}) to optimise it for Uncloud..."
    echo "${DOCKER_DAEMON_CONFIG}" > "${DOCKER_DAEMON_CONFIG_FILE}"

    if [[ "${INSTALL_ONLY}" != "true" ]]; then
        systemctl restart docker
    fi

    log "✓ Docker installed and configured successfully."
}

create_uncloud_user_and_group() {
    if id "${UNCLOUD_USER}" &> /dev/null; then
        log "✓ Linux user '${UNCLOUD_USER}' already exists."
    else
        # In addition to creating the user, create a group with the same name as the user.
        if ! useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin --user-group "${UNCLOUD_USER}"; then
            error "Failed to create Linux user '${UNCLOUD_USER}'."
        fi
        log "✓ Linux user and group '${UNCLOUD_USER}' created."
    fi

    if [ -n "${UNCLOUD_GROUP_ADD_USER}" ]; then
        if ! gpasswd --add "${UNCLOUD_GROUP_ADD_USER}" "${UNCLOUD_USER}" > /dev/null; then
            error "Failed to add user '${UNCLOUD_GROUP_ADD_USER}' to group '${UNCLOUD_USER}'."
        fi
        log "✓ Linux user '${UNCLOUD_GROUP_ADD_USER}' added to group '${UNCLOUD_USER}'."
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
        # TODO: Check the version of the installed uncloudd binary and update if there is a newer stable version.
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
    local uninstall_url
    if [ "${UNCLOUD_VERSION}" == "latest" ]; then
        uncloudd_url="${UNCLOUD_GITHUB_URL}/releases/latest/download/uncloudd_linux_${file_arch}.tar.gz"
        uninstall_url="https://raw.githubusercontent.com/psviderski/uncloud/refs/heads/main/scripts/uninstall.sh"
    else
        uncloudd_url="${UNCLOUD_GITHUB_URL}/releases/download/v${UNCLOUD_VERSION}/uncloudd_linux_${file_arch}.tar.gz"
        uninstall_url="https://raw.githubusercontent.com/psviderski/uncloud/refs/tags/v${UNCLOUD_VERSION}/scripts/uninstall.sh"
    fi
    local uncloudd_download_path="${tmp_dir}/uncloudd.tar.gz"
    local uninstall_download_path="${tmp_dir}/uninstall.sh"

    log "⏳ Downloading uncloudd binary: ${uncloudd_url}"
    if ! curl -fsSL -o "${uncloudd_download_path}" "${uncloudd_url}"; then
        error "Failed to download uncloudd binary."
    fi
    tar -xf "${uncloudd_download_path}" --directory "${tmp_dir}"
    if ! install "${tmp_dir}/uncloudd" "${uncloudd_install_path}"; then
        error "Failed to install uncloud binary to ${uncloudd_install_path}"
    fi
    log "✓ uncloudd binary installed: ${uncloudd_install_path}"

    log "⏳ Downloading uninstall script: ${uninstall_url}"
    if ! curl -fsSL -o "${uninstall_download_path}" "${uninstall_url}"; then
        error "Failed to download uninstall script."
    fi
    local uninstall_install_path="${INSTALL_BIN_DIR}/uncloud-uninstall"
    if ! install "${uninstall_download_path}" "${uninstall_install_path}"; then
        error "Failed to install uninstall.sh script to ${uninstall_install_path}"
    fi
    log "✓ uncloud-uninstall script installed: ${uninstall_install_path}"

    # TODO: install uncloud CLI binary and create a uc alias.
}

install_uncloud_systemd() {
    local uncloud_service_path="${INSTALL_SYSTEMD_DIR}/uncloud.service"
    cat > "${uncloud_service_path}" << EOF
[Unit]
Description=Uncloud machine daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=${INSTALL_BIN_DIR}/uncloudd
TimeoutStartSec=15
Restart=always
RestartSec=2

# Hardening options.
NoNewPrivileges=true
ProtectSystem=full
ProtectControlGroups=true
ProtectHome=read-only
ProtectKernelTunables=true
PrivateTmp=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
RestrictNamespaces=true

[Install]
WantedBy=multi-user.target
EOF
    log "✓ Systemd unit file created: ${uncloud_service_path}"


    if [[ "${INSTALL_ONLY}" != "true" ]]; then
        # Reload systemd to recognize the new or updated unit file.
        systemctl daemon-reload
    fi
    systemctl enable uncloud.service
}

install_corrosion() {
    local arch
    arch=$(uname -m)

    local corrosion_install_path="${INSTALL_BIN_DIR}/uncloud-corrosion"
    if [ -f "${corrosion_install_path}" ]; then
        # TODO: Check the version of the installed corrosion binary and update if there is a newer stable version.
        log "✓ uncloud-corrosion binary is already installed."
        return
    fi

    # Create a temporary directory for downloads.
    local tmp_dir
    tmp_dir=$(mktemp -d)
    # Ensure the temporary directory is deleted on script exit.
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp_dir'" EXIT

    local corrosion_url
    if [ "${CORROSION_VERSION}" == "latest" ]; then
        corrosion_url="${CORROSION_GITHUB_URL}/releases/latest/download/corrosion-${arch}-unknown-linux-gnu.tar.gz"
    else
        corrosion_url="${CORROSION_GITHUB_URL}/releases/download/${CORROSION_VERSION}/corrosion-${arch}-unknown-linux-gnu.tar.gz"
    fi
    local corrosion_download_path="${tmp_dir}/corrosion.tar.gz"

    log "⏳ Downloading uncloud-corrosion binary: ${corrosion_url}"
    if ! curl -fsSL -o "${corrosion_download_path}" "${corrosion_url}"; then
        error "Failed to download uncloud-corrosion binary."
    fi
    tar -xf "${corrosion_download_path}" -C "${tmp_dir}"
    if ! install "${tmp_dir}/corrosion" "${corrosion_install_path}"; then
        error "Failed to install uncloud-corrosion binary to ${corrosion_install_path}"
    fi
    log "✓ uncloud-corrosion binary installed: ${corrosion_install_path}"
}

install_corrosion_systemd() {
    local corrosion_service_path="${INSTALL_SYSTEMD_DIR}/uncloud-corrosion.service"
    cat > "${corrosion_service_path}" << EOF
[Unit]
Description=Uncloud gossip-based distributed store
PartOf=uncloud.service

[Service]
Type=simple
ExecStart=${INSTALL_BIN_DIR}/uncloud-corrosion agent -c ${UNCLOUD_DATA_DIR}/corrosion/config.toml
ExecReload=${INSTALL_BIN_DIR}/uncloud-corrosion reload -c ${UNCLOUD_DATA_DIR}/corrosion/config.toml
Restart=always
RestartSec=2
User=${UNCLOUD_USER}
Group=${UNCLOUD_USER}

# Hardening options.
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true
ProtectHome=true
ProtectControlGroups=true
ProtectKernelTunables=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
EOF
    log "✓ Systemd unit file created: ${corrosion_service_path}"

    if [[ "${INSTALL_ONLY}" != "true" ]]; then
        # Reload systemd to recognize the new unit file
        systemctl daemon-reload
    fi
}

start_uncloud() {
    if [[ "${INSTALL_ONLY}" == "true" ]]; then
        return
    fi

    log "⏳ Starting Uncloud machine daemon (uncloud.service)..."
    systemctl restart uncloud.service
    log "✓ Uncloud machine daemon started."
}

log "⏳ Running Uncloud install script..."

if [ "$EUID" -ne 0 ]; then
    error "Please run the install script with sudo or as root."
fi

verify_system
install_docker
create_uncloud_user_and_group
install_uncloud_binaries
install_uncloud_systemd
install_corrosion
install_corrosion_systemd
start_uncloud

# Show warning if Docker was already installed without containerd image store enabled.
if [ "$DOCKER_ALREADY_INSTALLED" = "true" ] && [ "$CONTAINERD_IMAGE_STORE_ENABLED" = "false" ]; then
    echo ""
    warning "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    warning "⚠️  IMPORTANT: Containerd image store configuration"
    warning "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    warning "Docker was already installed on the machine but it doesn't use the containerd"
    warning "image store. Uncloud works best with the containerd image store enabled in Docker."
    warning "It allows Docker to directly use the images stored in containerd (pushed with"
    warning "'uc image push') without duplicating them in Docker. This saves disk space and"
    warning "makes image management more efficient."
    echo ""
    warning "See https://docs.docker.com/engine/storage/containerd/ for more details."
    echo ""
    warning "To enable it, run the following commands on the machine:"
    echo ""
    echo "sudo bash -c 'cat > ${DOCKER_DAEMON_CONFIG_FILE} << EOF"
    echo "${DOCKER_DAEMON_CONFIG}"
    echo "EOF'"
    echo "sudo systemctl restart docker"
    echo ""
    warning "WARNING: Switching to containerd image store causes you to temporarily lose images"
    warning "and containers created using the classic storage driver. Those resources still"
    warning "exist on your filesystem, and you can retrieve them by turning off the containerd"
    warning "image store feature."
    warning "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
fi

log "✓ Uncloud installed on the machine successfully! 🎉"
