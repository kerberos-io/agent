#!/usr/bin/env bash
set -euo pipefail
image=${1:?usage: image/smoke.sh IMAGE}
test "$(docker image inspect "$image" --format '{{.Config.User}}')" = 65532:65532
test "$(docker image inspect "$image" --format '{{json .Config.Cmd}}')" = '["./main","-action","run"]'
test "$(docker image inspect "$image" --format '{{.Config.WorkingDir}}')" = /home/agent
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  --entrypoint /bin/sh "$image" -ec '
  test "$(id -u)" = 65532
  test "$(stat -c %u:%g /home/agent/main)" = 0:0
  test ! -w /home/agent/main
  test ! -w /home/agent/www
  test ! -e /usr/bin/apt
  test ! -e /usr/bin/dpkg
  test -z "$(find / -xdev -type f -perm /6000 -print 2>/dev/null)"
  ./main -action version
  ffmpeg -version
  ffprobe -version
  '
container=
volume=
cleanup() {
  if [[ -n "$container" ]]; then
    docker logs "$container" || true
    docker rm -f "$container" >/dev/null
  fi
  if [[ -n "$volume" ]]; then docker volume rm "$volume" >/dev/null; fi
}
trap cleanup EXIT
for mode in production demo; do
  volume=$(docker volume create)
  args=()
  port=80
  if [[ "$mode" == demo ]]; then
    port=8080
    args+=(-e AGENT_MODE=demo -e AGENT_PORT=8080)
  fi
  container=$(docker run -d --read-only --cap-drop ALL \
    --security-opt no-new-privileges \
    --sysctl net.ipv4.ip_unprivileged_port_start=0 \
    --mount "type=volume,src=$volume,dst=/home/agent/data" \
    -p "127.0.0.1::$port" "${args[@]}" "$image")
  address=$(docker port "$container" "$port/tcp")
  healthy=false
  for attempt in {1..60}; do
    if curl --fail --silent "http://$address/health" >/dev/null; then
      healthy=true
      break
    fi
    test "$(docker inspect "$container" --format '{{.State.Running}}')" = true
    sleep 2
  done
  test "$healthy" = true
  curl --fail --silent "http://$address/" | grep -q '<html'
  curl --fail --silent "http://$address/env.js" > "image-env-$mode.js"
  docker cp "$container:/home/agent/www/env.$([[ "$mode" == demo ]] && echo demo. || true)js" "image-expected-$mode.js"
  cmp "image-env-$mode.js" "image-expected-$mode.js"
  docker exec "$container" /bin/sh -ec '
    test -w data/config/config.json
    test -w data/config/user.json
    ffmpeg -nostdin -v error -i data/test-480p.mp4 -frames:v 1 data/capture-test/smoke.jpg
    test -s data/capture-test/smoke.jpg
    ffprobe -v error -show_entries stream=codec_type data/test-480p.mp4
    for path in cloud snapshots log recordings capture-test config tus; do
      test -w "data/$path"
    done
    test ! -w www/env.js
    '
  cleanup
  container=
  volume=
done
