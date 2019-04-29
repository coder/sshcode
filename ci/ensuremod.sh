#!/bin/bash

# inspired by nhooyr's days as CI overlord

set -eou pipefail

function help() {
  echo
  echo "you may need to update go.mod/go.sum via:"
  echo "go list all > /dev/null"
  echo "go mod tidy"
  exit 1
}

go list -mod=readonly all > /dev/null

go mod tidy

if [[ $(git diff --name-only) != "" ]]; then
  git diff
  help
fi
