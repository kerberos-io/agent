#!/bin/sh
set -eu

image=${1:?Usage: sh deployments/docker/test-image.sh IMAGE}
container=
volume=
cleanup() {
	if [ -n "$container" ]; then docker rm -f "$container" >/dev/null; fi
	if [ -n "$volume" ]; then docker volume rm "$volume" >/dev/null; fi
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
	test -w /home/agent/www
	test -z "$(find / -xdev -type f -perm /6000 -print 2>/dev/null)"
	! command -v setcap
	! command -v capsh
	test ! -e /home/agent/.env
	test -z "$(find /home/agent/data/tus -type f -print)"
	./main -action version
	ffmpeg -nostdin -v error -f lavfi -i sine=frequency=1000:sample_rate=8000 \
		-t 0.1 -c:a aac -f adts /tmp/test.aac
	ffmpeg -nostdin -v error -i /tmp/test.aac -ar 8000 -ac 1 \
		-c:a pcm_mulaw -f mulaw /tmp/test.pcm
	test -s /tmp/test.pcm
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

# Exercise the default port, Docker volume initialization, and reduced capabilities.
volume=$(docker volume create)
container=$(docker run -d --cap-drop ALL --cap-add NET_BIND_SERVICE \
	--sysctl net.ipv4.ip_unprivileged_port_start=1024 \
	--health-interval=1s --health-start-period=0s \
	--mount "type=volume,source=$volume,target=/home/agent/data" "$image")
wait_healthy
docker exec "$container" sh -ec '
	test -r data/config/config.json
	touch data/config/.permission-test data/recordings/.permission-test data/tus/.permission-test
	rm data/config/.permission-test data/recordings/.permission-test data/tus/.permission-test
'
docker stop --time 10 "$container" >/dev/null
docker rm "$container" >/dev/null
container=

# Demo mode must still be able to replace env.js; probes must bypass proxy settings.
container=$(docker run -d --cap-drop ALL --cap-add NET_BIND_SERVICE \
	--security-opt no-new-privileges=true \
	--sysctl net.ipv4.ip_unprivileged_port_start=1024 \
	--health-interval=1s --health-start-period=0s \
	-e AGENT_PORT=8082 -e AGENT_MODE=demo \
	-e http_proxy=http://127.0.0.1:9 -e HTTP_PROXY=http://127.0.0.1:9 \
	-e no_proxy= -e NO_PROXY= "$image")
wait_healthy
docker exec "$container" sh -ec 'test -f www/env.js; test ! -e www/env.demo.js'
echo "Image permissions, media transcoding, volume writes, default/custom ports, and demo startup passed."
