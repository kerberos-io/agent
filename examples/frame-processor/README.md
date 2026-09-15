# Example Frame Processor

This reference service defines and exercises the Kerberos Agent frame-processing
contract. It accepts Agent JPEGs over HTTP, makes a deterministic decision, and
publishes Agent control commands over MQTT. It has no RabbitMQ or machine-learning
runtime dependency.

## Endpoints

- `GET /health`
- `POST /v1/frames` with multipart `metadata` JSON and `frame` JPEG parts
- `POST /v1/frame-requests` with JSON to request capture from one or more Agents

See [openapi.yaml](openapi.yaml) and [MQTT.md](MQTT.md) for the versioned wire
contracts.

## Run

```bash
export FRAME_PROCESSOR_API_TOKEN=development-token
export FRAME_PROCESSOR_MQTT_URI=tcp://localhost:1883
export FRAME_PROCESSOR_HUB_KEY=development-hub
export FRAME_PROCESSOR_PROFILE=never-trigger
go run .
```

The broker credentials are optional when the local broker permits anonymous
connections:

```bash
export FRAME_PROCESSOR_MQTT_USERNAME=...
export FRAME_PROCESSOR_MQTT_PASSWORD=...
```

Request a frame from an Agent:

```bash
curl --fail-with-body \
  -H 'Authorization: Bearer development-token' \
  -H 'Content-Type: application/json' \
  --data @testdata/frame-request.json \
  http://localhost:8080/v1/frame-requests
```

## Profiles

- `never-trigger`
- `always-trigger`
- `every-nth-frame`
- `brightness-threshold`

Use `FRAME_PROCESSOR_EVERY_N` and `FRAME_PROCESSOR_BRIGHTNESS_THRESHOLD` to tune
the last two profiles. `FRAME_PROCESSOR_DELAY_MILLISECONDS` and
`FRAME_PROCESSOR_FORCE_ERROR` provide deterministic latency and failure
simulation.

Recording commands default to a 30-second event clip with 10 seconds of pre-roll.
Configure them with `FRAME_PROCESSOR_EVENT_CLIP_SECONDS` and
`FRAME_PROCESSOR_PRE_ROLL_SECONDS`.

The reference MQTT publisher emits plaintext Agent envelopes for local contract
testing. Use a trusted broker. Production support for untrusted brokers requires
the same encrypted-message packaging used by Hub and Agent.

Frame idempotency is guaranteed until the submitted frame's `expiresAt`. The
service rejects frame TTLs longer than `FRAME_PROCESSOR_MAX_FRAME_TTL_SECONDS`
(five minutes by default), then evicts the cached result at expiry.

## Verify

```bash
GOWORK=off go test ./...
GOWORK=off go vet ./...
```