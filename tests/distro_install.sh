#!/usr/bin/env bash
# Disposable, privileged CI container only; never run the inner test on a VPS.
set -euo pipefail
[[ ${GITHUB_ACTIONS:-} == true ]] || { echo 'Requires a disposable GitHub runner'; exit 1; }
image=${1:?base image required}
root=$(cd "$(dirname "$0")/.." && pwd)
command -v docker >/dev/null
docker build --build-arg "BASE_IMAGE=$image" -f "$root/tests/distro.Dockerfile" -t xnvgatebox-distro-check "$root/tests"
name=xnvgatebox-distro-$RANDOM
trap 'docker logs "$name"; docker rm -f "$name" >/dev/null' EXIT
docker run -d --name "$name" --privileged --cgroupns=host \
  --tmpfs /run --tmpfs /run/lock --tmpfs /tmp \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  -v "$root:/src:ro" -v "$root/dist:/package:ro" xnvgatebox-distro-check
for attempt in {1..30}; do
  if docker exec "$name" systemctl show --property=Version --value >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$name" bash /src/tests/distro_install_inner.sh
