#!/usr/bin/env bash
set -euo pipefail

gate=scripts/validate-container-release.sh

bash "$gate" release published false v1.2.3

while IFS='|' read -r event_name release_action prerelease version; do
  if bash "$gate" "$event_name" "$release_action" "$prerelease" "$version"; then
    echo "release gate accepted $event_name $release_action $prerelease $version" >&2
    exit 1
  fi
done <<'CASES'
push|published|false|v1.2.3
pull_request|published|false|v1.2.3
release|created|false|v1.2.3
release|published|true|v1.2.3
release|published|false|1.2.3
release|published|false|v1.2
release|published|false|v1.2.3-rc.1
release|published|false|v1.2.3.4
CASES
