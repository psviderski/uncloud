#!/bin/sh
set -eu

dind dockerd &
echo "Waiting for Docker in Docker to be ready..."
timeout 5s sh -c "until docker info &> /dev/null; do sleep 0.1; done"
echo "Docker in Docker is ready."

echo "Loading corrosion image from /images/corrosion.tar..."
docker load < /images/corrosion.tar

exec "$@"
