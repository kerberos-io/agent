#!/bin/sh
set -eu

image=${1:?Usage: sh deployments/docker/test-image.sh IMAGE}
container=
volume=
temporary_volume=
cleanup() {
	if [ -n "$container" ]; then docker rm -f "$container" >/dev/null; fi
	if [ -n "$volume" ]; then docker volume rm "$volume" >/dev/null; fi
	if [ -n "$temporary_volume" ]; then docker volume rm "$temporary_volume" >/dev/null; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

docker run --rm --entrypoint /bin/sh "$image" -ec '
	test "$(id -u)" = 999
	test "$(id -g)" = 999
	test ! -w /home/agent
	test ! -w /home/agent/main
	test -w /home/agent/data/config
	test -w /home/agent/data/recordings
	test -w /home/agent/data/tus
	test ! -w /home/agent/www
	test ! -w /home/agent/www/env.js
	test -z "$(find / -xdev -type f -perm /6000 -print 2>/dev/null)"
	! command -v setcap
	! command -v capsh
	! command -v apt
	! command -v apt-get
	! command -v dpkg
	! command -v curl
	test -s /var/lib/dpkg/status
	test -r /etc/os-release
	test -r /usr/share/terminfo/x/xterm-256color
	test -s /etc/ssl/certs/ca-certificates.crt
	test "$(id -Gn)" = "kerberosio"
	test "$(stat -c %a /tmp)" = 1777
	test -w "$XDG_CACHE_HOME"
	test ! -e /home/agent/.env
	test -z "$(find /home/agent/data/tus -type f -print)"
	./main -action version
	ffmpeg -nostdin -v error -f lavfi -i sine=frequency=1000:sample_rate=8000 \
		-t 0.1 -c:a aac -f adts /tmp/test.aac
	ffmpeg -nostdin -v error -i /tmp/test.aac -ar 8000 -ac 1 \
		-c:a pcm_mulaw -f mulaw /tmp/test.pcm
	test -s /tmp/test.pcm
	ffprobe -v error -show_entries stream=codec_name data/test-480p.mp4
	ffmpeg -nostdin -v error -i data/test-480p.mp4 -frames:v 1 -f null -
	ffmpeg -nostdin -v error -i data/test-480p.mp4 -frames:v 1 \
		-c:v libx265 -x265-params pools=none:frame-threads=1:log-level=error \
		-f hevc /tmp/test.hevc
	ffmpeg -nostdin -v error -i /tmp/test.hevc -frames:v 1 -f null -
	ffmpeg -nostdin -v error -f lavfi -i color=black:s=64x64 \
		-vf drawtext=text=scratch:fontcolor=white -frames:v 1 -f null -
'

# Exercise the same shell and PTY devices used by remote-access sessions.
docker run --rm -t --entrypoint /bin/sh "$image" -ec '
	test -t 0
	test -t 1
	test -c /dev/ptmx
	test -d /dev/pts
	stty size
	printf "Shell and PTY ready\n"
'
docker run --rm --group-add 44 --entrypoint /bin/sh "$image" -ec '
	test "$(id -Gn)" = "kerberosio video"
'
wait_healthy() {
	attempt=0
	while [ "$attempt" -lt 60 ]; do
		status=$(docker inspect --format '{{.State.Health.Status}}' "$container")
		if [ "$status" = healthy ]; then return; fi
		if [ "$(docker inspect --format '{{.State.Running}}' "$container")" != true ]; then
			break
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	docker logs "$container"
	docker inspect --format '{{json .State.Health}}' "$container"
	echo "Agent did not become healthy" >&2
	return 1
}

# Exercise the unprivileged default port without any capabilities.
volume=$(docker volume create)
container=$(docker run -d --cap-drop ALL --security-opt no-new-privileges=true \
	--sysctl net.ipv4.ip_unprivileged_port_start=1024 \
	--health-interval=1s --health-start-period=0s \
	--mount "type=volume,source=$volume,target=/home/agent/data" "$image")
wait_healthy
docker exec "$container" sh -ec '
	test "$AGENT_PORT" = 8080
	./main -action healthcheck
	test -r data/config/config.json
	touch data/config/.permission-test data/recordings/.permission-test data/tus/.permission-test
	rm data/config/.permission-test data/recordings/.permission-test data/tus/.permission-test
'
if docker exec -e AGENT_PORT=65534 "$container" ./main -action healthcheck; then
	echo "Healthcheck incorrectly succeeded on a port with no Agent" >&2
	exit 1
fi
docker stop --time 10 "$container" >/dev/null
docker rm "$container" >/dev/null
container=

# Demo UI selection must work with immutable assets and a read-only root.
temporary_volume=$(docker volume create)
container=$(docker run -d --read-only --cap-drop ALL \
	--security-opt no-new-privileges=true \
	--sysctl net.ipv4.ip_unprivileged_port_start=1024 \
	--health-interval=1s --health-start-period=0s \
	--mount "type=volume,source=$volume,target=/home/agent/data" \
	--mount "type=volume,source=$temporary_volume,target=/tmp" \
	-e AGENT_PORT=8082 -e AGENT_MODE=demo \
	-e http_proxy=http://127.0.0.1:9 -e HTTP_PROXY=http://127.0.0.1:9 \
	-e no_proxy= -e NO_PROXY= "$image")
wait_healthy
docker exec "$container" sh -ec '
	test -f www/env.js
	test -f www/env.demo.js
	test ! -w www
	ffmpeg -nostdin -v error -f lavfi -i color=black:s=64x64 \
		-vf drawtext=text=readonly:fontcolor=white -frames:v 1 -f null -
'
echo "Scratch image permissions, media, shell/PTY, volume writes, default/custom ports, and demo startup passed."
