#!/bin/sh
set -eu

# Cleanup function to properly terminate background processes.
cleanup() {
    echo "Terminating container processes..."

    # Terminate the main process if it has been started.
    if [ -n "${MAIN_PID:-}" ]; then
        kill "$MAIN_PID" 2>/dev/null || true
    fi

    # Terminate Docker daemon if PID file exists.
    if [ -f /run/docker.pid ]; then
        kill "$(cat /run/docker.pid)" 2>/dev/null || true
    fi

    # Terminate socat proxy if running.
    pkill socat 2>/dev/null || true

    # Wait for processes to terminate.
    wait
}
trap cleanup INT TERM EXIT

# Remove stale Docker and containerd pid files left over from the previous container run. They persist
# across restarts because /run lives in the container's overlay filesystem, not a tmpfs.
rm -f /run/docker.pid /run/docker/containerd/containerd.pid

dind dockerd &
echo "Waiting for Docker in Docker to be ready..."
timeout 60s sh -c "until docker info &> /dev/null; do sleep 0.5; done"
echo "Docker in Docker is ready."

echo "Loading corrosion image from /images/corrosion.tar..."
docker load < /images/corrosion.tar

# Make machine API accessible from the host via port publishing.
echo "Proxying Uncloud API port 51000/tcp to Unix socket /run/uncloud/uncloud.sock..."
socat TCP-LISTEN:51000,reuseaddr,fork,bind="$(hostname -i)" UNIX-CONNECT:/run/uncloud/uncloud.sock &

# Execute the passed command and wait for it while maintaining signal handling.
"$@" &
MAIN_PID=$!
wait $MAIN_PID
