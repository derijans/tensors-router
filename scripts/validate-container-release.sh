#!/usr/bin/env bash
set -euo pipefail

event_name=${1:?event name is required}
release_action=${2:?release action is required}
prerelease=${3:?prerelease flag is required}
version=${4:?version is required}

[[ "$event_name" == "release" ]]
[[ "$release_action" == "published" ]]
[[ "$prerelease" == "false" ]]
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
