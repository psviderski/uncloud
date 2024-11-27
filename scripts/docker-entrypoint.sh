#!/bin/sh
set -eu

dind dockerd &

exec "$@"
