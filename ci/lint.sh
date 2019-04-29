#!/bin/bash

# Inspired by nhooyr's days as CI overlord.

set -euo pipefail

files=$(gofmt -l -s .)

if [ ! -z "$files" ]; 
then
  echo "The following files need to be formatted:"
  echo "$files"
  echo "Please run 'gofmt -w -s .'"
  exit 1
fi

go vet -composites=false .
