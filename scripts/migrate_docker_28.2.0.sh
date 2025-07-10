#!/bin/bash
# This script migrates the Docker network 'uncloud' on a machine that was created with uncloud <0.9.0 to be compatible
# with Docker 28.2.0+. This fixes cross-machine communication for uncloud service containers.
# It disconnects all containers from the network, removes the network, and then recreates it with the new configuration.
# You should run this script as root or with sudo privileges.
set -euo pipefail

# Do not recreate the network if it already has the required configuration.
TRUSTED_INTERFACES=$(docker network inspect uncloud -f '{{index .Options "com.docker.network.bridge.trusted_host_interfaces"}}' 2>/dev/null)
if [ "$TRUSTED_INTERFACES" = "uncloud" ]; then
    echo "Network uncloud already has the required option com.docker.network.bridge.trusted_host_interfaces=uncloud."
    echo "No migration needed."
    exit 0
fi

echo "Getting containers connected to Docker network uncloud..."
CONTAINERS=$(docker network inspect uncloud -f '{{range .Containers}}{{.Name}} {{end}}' 2>/dev/null)
if [ -z "$CONTAINERS" ]; then
    echo "No containers found connected to uncloud network."
else
    echo "Found containers: $CONTAINERS"
fi

SUBNET=$(docker network inspect uncloud -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null)
if [ -z "$SUBNET" ]; then
    echo "Error: Could not get subnet for Docker network uncloud."
    exit 1
fi
echo "Current uncloud network subnet: $SUBNET"

if [ -n "$CONTAINERS" ]; then
    echo "Disconnecting containers from uncloud network..."
    for CONTAINER in $CONTAINERS; do
        echo "  Disconnecting $CONTAINER..."
        docker network disconnect uncloud "$CONTAINER"
    done
fi

echo "Deleting uncloud network..."
docker network rm uncloud

echo "Recreating uncloud network with new configuration..."
if docker network create \
    --subnet "$SUBNET" \
    --label uncloud.managed \
    -o com.docker.network.bridge.trusted_host_interfaces=uncloud \
    uncloud;
then
    echo "Network uncloud successfully recreated."
else
    echo "Error: Failed to recreate uncloud network."
    exit 1
fi

if [ -n "$CONTAINERS" ]; then
    echo "Reconnecting containers to uncloud network..."
    for CONTAINER in $CONTAINERS; do
        echo "  Reconnecting $CONTAINER..."
        docker network connect uncloud $CONTAINER
    done
fi

echo "Restarting uncloud machine daemon..."
systemctl restart uncloud.service

echo "Done!"
