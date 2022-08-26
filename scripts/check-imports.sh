#!/bin/bash

readonly SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly REPO_ROOT="${SCRIPT_DIR}/.."
readonly GOIMPORTS="${REPO_ROOT}/bin/goimports"

module_name="$(awk '{if ($1 == "module") {print $2}}' go.mod)"

if [[ -n "$($GOIMPORTS --local $module_name -l .)" ]]; then
  $GOIMPORTS --local $module_name -d .
  exit 1
fi
