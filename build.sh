#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd)"
DIST_DIR="$SCRIPT_DIR/dist"

function error() { echo -e "ERROR: $*$" 1>&2; }
function fatal() { error "$*"; exit 1; }
function invalid_argument() {
  error "$1 [$2]"
  echo ""
  usage
  exit 1
}

function usage() {
    echo "Runs a docker build"
    echo "Usage: $0 [OPTIONS]"
    printf "    %-30s enables verbose logging (-vvv for more, -vvvv to enable script debugging)\n" "-v, --verbose"
    printf "    %-30s display this help message\n" "-h, --help"
}

function cleanup() {
  rm -rf "$DIST_DIR"
}

function build() {
  cleanup
  run_docker "$@" || fatal "failed while running docker: $?"
}

function run_docker() {
  local dockerfile_path="$SCRIPT_DIR/Dockerfile"
  if [ ! -f "$dockerfile_path" ]; then
    fatal "Dockerfile '$dockerfile_path' does not exist"
  fi
  local dockerfile_header_regex="#[[:space:]]*syntax[[:space:]]*=[[:space:]]*docker\/dockerfile:([[:digit:]]+(\.[[:digit:]]+)*-)?experimental"
  local dockerfile_header=""
  dockerfile_header="$(head -n1 "$dockerfile_path")" || fatal "failed to read header from Dockerfile '$dockerfile_path'"
  if [[ ! "$dockerfile_header" =~ $dockerfile_header_regex ]]; then
    fatal "Dockerfile '$dockerfile_path' does not contain the required syntax header.  See: https://docs.docker.com/develop/develop-images/build_enhancements/"
  fi
  local progress_type="auto"
  local ssh_auth_sock="$SSH_AUTH_SOCK"
  if [[ "$verbose_count" -ge 1 ]]; then
    progress_type="plain"
  fi
  local ssh_flag=""
  if [[ "$ssh_auth_sock" != "" ]]; then
    ssh_flag=" --ssh default=$ssh_auth_sock"
  else
    warn "SSH_AUTH_SOCK is not configured.  SSH Agent forwarding is disabled."
  fi
  docker buildx build \
    --progress $progress_type \
    --output type=local,dest="${SCRIPT_DIR}"${ssh_flag} \
    --file "$dockerfile_path" \
    --build-arg "VERSION=$(git describe --tags --candidates=1 --dirty)" \
    --build-arg "REVISION=$(git rev-parse HEAD)" \
    --build-arg "BRANCH=$(git rev-parse --abbrev-ref HEAD)" \
    --build-arg "USER=$USER" \
    --build-arg "HOST=$(hostname)" \
    --build-arg "BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    "$SCRIPT_DIR" || fatal "build failed: $?"
}

function parse_args() {
  while [[ $# -gt 0 ]]; do
    local key="$1"
    case $key in
      --verbose)
        verbose_count=1
        return 0
      ;;
      -v | -vv | -vvv | -vvvv)
        local verbose_flags="${key##-}"
        verbose_count="${#verbose_flags}"
        if [[ "$verbose_count" -ge 4 ]]; then
          set -x
        fi
      ;;
      -h | --help)
        usage
        exit 0
      ;;
      *)
        invalid_argument "unsupported input parameter" "$key"
      ;;
    esac
    shift 1
  done
}

function run() {
  parse_args "$@"

  if ! docker version > /dev/null 2>&1; then
    fatal "docker is not installed or docker daemon is not running"
  fi

  if ! docker info -f "{{json .}}" | jq -re '.ClientInfo.Plugins[] | select(.Name=="buildx")' > /dev/null 2>&1; then
    fatal "docker cli experimental mode is not enabled.  See https://docs.docker.com/buildx/working-with-buildx/ for instructions."
  fi

  if ! docker info -f "{{json .}}" | jq -re '.ExperimentalBuild' > /dev/null 2>&1; then
    fatal "docker experimental mode is not enabled.  See https://docs.docker.com/buildx/working-with-buildx/ for instructions."
  fi

  build "$@"

  return $?
}

run "$@"
