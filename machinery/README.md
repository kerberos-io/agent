# Kerberos Open Source - Machinery

<a target="_blank" href="https://kerberos.io"><img src="https://img.shields.io/badge/kerberos-website-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>
<a target="_blank" href="https://doc.kerberos.io"><img src="https://img.shields.io/badge/kerberos-documentation-gray.svg?longCache=true&colorB=brightgreen" alt="Kerberos Agent"></a>
<a target="_blank" href="https://twitter.com/kerberosio?ref_src=twsrc%5Etfw"><img src="https://img.shields.io/twitter/url.svg?label=Follow%20%40kerberosio&style=social&url=https%3A%2F%2Ftwitter.com%2Fkerberosio" alt="Twitter Widget"></a>
<a target="_blank" href="https://join.slack.com/t/kerberosio/shared_invite/zt-1a5oj4pwm-O4qCAN9c5r2um0Ns0ge8ww"><img src="https://img.shields.io/badge/slack-@kerberosio-yellow.svg?logo=slack " alt="Kerberos.io"></a>

<img src="https://github.com/kerberos-io/agent/workflows/Go/badge.svg"/>
<img src="https://github.com/kerberos-io/agent/workflows/CodeQL/badge.svg"/>

<a target="_blank" href="https://pkg.go.dev/github.com/kerberos-io/agent/machinery"><img src="https://pkg.go.dev/badge/github.com/kerberos-io/agent/machinery" alt="PkgGoDev"></a>
<a target="_blank" href="https://codecov.io/gh/kerberos-io/agent"><img src="https://codecov.io/gh/kerberos-io/agent/branch/master/graph/badge.svg" alt="Coverage Status"></a>
<a target="_blank" href="https://goreportcard.com/report/github.com/kerberos-io/agent/machinery"><img src="https://goreportcard.com/badge/github.com/kerberos-io/agent/machinery" alt="Coverage Status"></a>
<a target="_blank" href="https://app.codacy.com/gh/kerberos-io/agent?utm_source=github.com&utm_medium=referral&utm_content=kerberos-io/agent&utm_campaign=Badge_Grade"><img src="https://api.codacy.com/project/badge/Grade/83d79d3092c040acb8c51ee0dfddf4b9"/>
<a target="_blank" href="https://www.figma.com/proto/msuYC6sv2cOCqZeDtBxNy7/%5BNEW%5D-Kerberos.io-Apps?node-id=1%3A1788&viewport=-490%2C191%2C0.34553584456443787&scaling=min-zoom&page-id=1%3A2%3Ffuid%3D449684443467913607" alt="Kerberos Agent"></a>

<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
[![donate](
https://brianmacdonald.github.io/Ethonate/svg/eth-donate-blue.svg)](
https://brianmacdonald.github.io/Ethonate/address#0xf4a759C9436E2280Ea9cdd23d3144D95538fF4bE)

[**Docker Hub**](https://hub.docker.com/r/kerberos/agent) | [**Documentation**](https://doc.kerberos.io) | [**Website**](https://kerberos.io)

Kerberos Open source (v3) is a cutting edge video surveillance management system made available as Open Source under the MIT License. This means that all the source code is available for you or your company, and you can use, transform and distribute the source code; as long you keep a reference of the original license. Kerberos Open Source (v3) can be used for commercial usage (which was not the case for v2). Read more [about the license here](LICENSE).

## Security reporting

For sensitive vulnerabilities, use private disclosure channels documented in [../SECURITY.md](../SECURITY.md).

## WebRTC startup and keyframe recovery

Each H264 viewer waits until its WebRTC transport is connected and a fresh IDR
arrives before receiving video. Reconnects, viewer PLI/FIR feedback, and a full
per-viewer sample buffer reset that viewer's keyframe gate without stalling
other viewers. Audio continues to use the existing independent sample queues.

The main and sub streams request keyframes from their own RTSP sources. Viewer
PLI and FIR feedback is translated into an RTSP PLI using the camera session's
SSRCs, never forwarded with the browser's SSRCs. Requests are coalesced and
limited to one per second per broadcaster, retrying while connected viewers
still need an IDR. Requests stop once all waiting viewers receive an IDR or
the broadcaster closes.

Camera support for RTCP PLI varies. Unsupported sources, cameras that ignore
feedback, and failed requests fall back to the next periodic IDR; failures are
logged as `keyframe_request_failed`. No encoder settings are changed. A camera
with a long GOP can therefore still start slowly; configure a shorter IDR
interval (for example 1-2 seconds) when camera feedback is unavailable, accounting
for the bandwidth tradeoff. An isolated cached IDR must not be replayed followed
by arbitrary live P-frames: the dependent reference chain would be incomplete.
