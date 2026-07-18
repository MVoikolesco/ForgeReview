#!/bin/sh
set -eu

# Named volumes are mounted after image layers and initially belong to root.
# Normalize only ForgeReview's writable directories, then drop privileges.
mkdir -p /data
chown -R app:app /data
exec su-exec app:app "$@"
