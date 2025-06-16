#!/usr/bin/env bash
# Install Protocol Buffers compiler, includes, and Go plugins.
set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

TOOLS_ROOT="${SCRIPT_DIR}/../_tools"
TOOLS_BIN="${TOOLS_ROOT}/bin"
TOOLS_INCLUDE="${TOOLS_ROOT}/include"
TMP_DIR="${TOOLS_ROOT}/tmp"

# Cleanup hook
cleanup() {
    echo "Cleaning up temporary files..."
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT ERR

install_protoc() {
    PROTOC_VERSION="27.3"
    PB_REL="https://github.com/protocolbuffers/protobuf/releases"

    echo "Installing protoc version ${PROTOC_VERSION}..."

    ARCH_RAW=$(uname -m)
    if [[ "${ARCH_RAW}" == "x86_64" ]]; then
        ARCH="x86_64"
    elif [[ "${ARCH_RAW}" == "aarch64" ]]; then
        ARCH="aarch_64"
    else
        echo "Unsupported architecture: ${ARCH_RAW}"
        exit 1
    fi

    OS_RAW=$(uname -s)
    if [[ "${OS_RAW}" == "Linux" ]]; then
        OS="linux"
    elif [[ "${OS_RAW}" == "Darwin" ]]; then
        OS="osx"
    else
        echo "Unsupported operating system: ${OS_RAW}"
        exit 1
    fi

    # Download the binary
    curl -L -o "${TMP_DIR}/protoc-current.zip" \
        "${PB_REL}/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-${OS}-${ARCH}.zip"
    unzip "${TMP_DIR}/protoc-current.zip" -d "${TMP_DIR}"
    mv "${TMP_DIR}/bin/protoc" "${TOOLS_BIN}/"

    # Download the includes
    curl -L -o "${TMP_DIR}/protobuf-src.tar.gz" \
        "${PB_REL}/download/v${PROTOC_VERSION}/protobuf-${PROTOC_VERSION}.tar.gz"
    tar -xzf "${TMP_DIR}/protobuf-src.tar.gz" -C "${TMP_DIR}"
    mv "${TMP_DIR}/protobuf-${PROTOC_VERSION}/src/google" "${TOOLS_INCLUDE}/"

    # TODO: version pinning and/or checksum verification

    # Delete all non-proto files
    find "${TOOLS_INCLUDE}/google/protobuf/" -type f ! -name "*.proto" -exec rm -f {} +
}

install_protoc_gen_go() {
    PROTOC_GEN_GO_VERSION="v1.34.2"
    echo "Installing protoc-gen-go version ${PROTOC_GEN_GO_VERSION}..."
    GOBIN="${TOOLS_BIN}" go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}

    PROTOC_GEN_GO_GRPC_VERSION="v1.5.1"
    echo "Installing protoc-gen-go-grpc version ${PROTOC_GEN_GO_GRPC_VERSION}..."
    GOBIN="${TOOLS_BIN}" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

    # TODO: version pinning and/or checksum verification
}

rm -rf "${TOOLS_ROOT}"
mkdir -p "${TOOLS_ROOT}" "${TOOLS_BIN}" "${TOOLS_INCLUDE}" "${TMP_DIR}"

install_protoc
install_protoc_gen_go
