#!/usr/bin/env bash

function run() {
  if [[ "${DEVBOX_SHELL_ENABLED:-0}" != "1" ]]; then
    devbox run build "$@"
    return $?
  fi

  export ENABLE_S3_UPLOAD="true"
  export DIST_DIR="$DEVBOX_PROJECT_ROOT/dist"

  export VERSION="$(git describe --tags --candidates=1 --dirty)"
  export REVISION="$(git rev-parse HEAD)"
  export BRANCH="$(git rev-parse --abbrev-ref HEAD)"
  export USER="${USER}"
  export HOST="$(hostname)"
  export BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  just build || exit 1
}

run "$@"
