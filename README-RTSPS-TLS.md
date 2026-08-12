# RTSPS and TLS certificates

This guide explains how Kerberos Agent connects to an IP camera over RTSPS,
how to issue a camera certificate with a private CA, and how to validate the
complete trust path. It also explains why some apparently corrupted trust
bundles can still allow a connection.

The camera-specific steps were verified with a Bosch FLEXIDOME micro 3100i.
Other Bosch firmware versions may use different labels or ports.

The commands were tested with Smallstep CLI `0.30.6` and OpenSSL `3.5.6` on
Debian. Check `step certificate sign --help` when using an older Smallstep CLI.
The OpenSSL isolation flags `-no-CApath` and `-no-CAstore` require a version that
lists them in `openssl s_client -help`.

## Tested configuration

| Setting | Value |
| --- | --- |
| Camera | Bosch FLEXIDOME micro 3100i |
| Example camera address | `10.0.30.11` |
| RTSPS port | `9554` |
| Main stream | `rtsps://<user>:<password>@10.0.30.11:9554/?inst=1` |
| Sub stream | `rtsps://<user>:<password>@10.0.30.11:9554/?inst=2` |
| Certificate SAN | `IP Address:10.0.30.11` |
| Bosch certificate usage | `HTTPS` |
| Agent trust input | Issuing intermediate plus root CA |

Replace the example address and certificate names throughout this guide. Keep
camera credentials out of source control and percent-encode reserved URL
characters in usernames and passwords.

## Mental model

### RTSPS, SRTSP, TLS, and SRTP

- The standard URL scheme is `rtsps://`. Do not use `srtsp://`.
- Bosch interfaces and documentation may use SRTSP or Secure RTSP as product
  terminology.
- RTSPS carries the RTSP control connection over TLS. With gortsplib, media is
  normally interleaved over the same TCP/TLS connection for this camera.
- SRTP is a separate media protection mechanism and is negotiated only when the
  camera advertises an appropriate secure RTP profile.

Encryption alone does not prove which camera the Agent reached. Verified TLS
also checks that:

1. The camera certificate is signed by a trusted authority.
2. The certificate is valid at the current time.
3. The URL host matches a certificate Subject Alternative Name (SAN).

Modern Go verification uses SANs for identity. A Common Name alone is not
sufficient. Connecting to `10.0.30.11` requires an IP SAN with that exact value,
not `DNS:10.0.30.11` and not only a device-name DNS SAN.

### Agent behavior

Kerberos Agent uses gortsplib for RTSP and RTSPS. With the normal configuration,
gortsplib receives a nil custom TLS configuration and Go performs standard
certificate and hostname verification with the process trust pool.

`AGENT_CAPTURE_IPCAMERA_RTSPS_INSECURE=true` is an explicit escape hatch that
sets `InsecureSkipVerify` for camera clients. It should be false in a verified
deployment.

## Decide the certificate identity first

Choose the stable name used in every Agent URL before creating the certificate:

- For an IP URL, add that address as an IP SAN.
- For a DNS URL, add the exact hostname as a DNS SAN.
- Add both when clients legitimately use both forms.

A certificate stops matching if the camera IP changes. Use a static address,
DHCP reservation, or stable DNS name.

## Configure RTSPS in the Bosch UI

1. Sign in to the camera as an administrator.
2. Open **Configuration**.
3. Open **Network > Network Services**.
4. Enable **RTSPS**.
5. Confirm port `9554`, or record the configured alternative.
6. Click **Set**.

RTSP on port `554` and RTSPS on port `9554` are separate services. Enabling
RTSPS does not make an `rtsp://` URL secure.

## Generate the private key and CSR on the camera

Keeping the TLS private key on the camera avoids exporting it to an operator
workstation or deployment system.

1. Open **Service > Certificates**.
2. Click **Add**.
3. Select **Generate signing request**.
4. Select `RSA 2048bit` or the stronger option supported by all clients.
5. Enter a unique file name, such as `agent-rtsps`.
6. Enter a descriptive Common Name and any required organization fields.
7. Click **Generate**.
8. Download the resulting CSR from the certificate table.

On the tested firmware, this form contains no SAN field. The downloaded CSR
therefore has no IP SAN. The CA must add the SAN while signing.

Inspect the CSR before signing:

```bash
openssl req -in camera.csr.pem -noout -verify -subject
openssl req -in camera.csr.pem -noout -text
```

The first command must report `Certificate request self-signature verify OK`.
An absent `Subject Alternative Name` section is expected for this firmware.

## Prepare Smallstep

Use an existing organizational CA when one is available. Creating a new CA
creates a new long-lived trust domain that must be distributed, protected,
backed up, and eventually rotated.

### Install the CLI on Debian amd64

```bash
curl -fsSL \
  https://dl.smallstep.com/cli/docs-ca-install/latest/step-cli_amd64.deb \
  -o /tmp/step-cli_amd64.deb
sudo dpkg -i /tmp/step-cli_amd64.deb
rm /tmp/step-cli_amd64.deb
step version
```

Use the official package matching the host architecture on other systems.

### Create a dedicated offline CA

Skip this section when using an existing CA.

```bash
umask 077
mkdir -p "$HOME/.step/secrets" "$HOME/.step/camera"

openssl rand -base64 48 > "$HOME/.step/secrets/camera_ca_password"
chmod 600 "$HOME/.step/secrets/camera_ca_password"

step ca init \
  --pki \
  --name "UUG Camera CA" \
  --password-file "$HOME/.step/secrets/camera_ca_password"
```

This produces:

```text
$HOME/.step/certs/root_ca.crt
$HOME/.step/certs/intermediate_ca.crt
$HOME/.step/secrets/root_ca_key
$HOME/.step/secrets/intermediate_ca_key
$HOME/.step/secrets/camera_ca_password
```

The files under `secrets/` are sensitive. Keep them mode `600`, never commit
them, and back them up to encrypted persistent storage. A devcontainer can be
rebuilt or deleted; it is not sufficient as the only CA backup.

## Add the SAN while signing

Copy the camera CSR into a protected working directory:

```bash
cp camera.csr.pem "$HOME/.step/camera/camera.csr.pem"
```

Create `$HOME/.step/camera/bosch-rtsps.tpl`:

```json
{
  "subject": {
    "commonName": {{ toJson .Insecure.CR.Subject.CommonName }}
  },
  "ipAddresses": ["10.0.30.11"],
  "keyUsage": ["keyEncipherment", "digitalSignature"],
  "extKeyUsage": ["serverAuth", "clientAuth"]
}
```

The template preserves the camera CSR public key, sets the IP identity, and
creates a TLS leaf rather than a CA certificate.

Sign it with a validity period that ends before the intermediate CA expires.
A one-year leaf is preferable to a ten-year leaf when automated renewal is
available:

```bash
step certificate sign \
  --template "$HOME/.step/camera/bosch-rtsps.tpl" \
  --bundle \
  --not-after 8760h \
  --password-file "$HOME/.step/secrets/camera_ca_password" \
  "$HOME/.step/camera/camera.csr.pem" \
  "$HOME/.step/certs/intermediate_ca.crt" \
  "$HOME/.step/secrets/intermediate_ca_key" \
  > "$HOME/.step/camera/bosch-rtsps-chain.pem"
```

For an online `step-ca`, do not assume `step ca sign` accepts a `--san` flag. It
does not. Authorize SANs in the one-time token or configure a provisioner
template that produces the required SANs.

## Validate before upload

Inspect the leaf certificate, which is the first PEM block in the chain file:

```bash
openssl x509 \
  -in "$HOME/.step/camera/bosch-rtsps-chain.pem" \
  -noout -subject -issuer -dates -ext subjectAltName -ext extendedKeyUsage
```

Confirm the SAN separately because some OpenSSL versions display only the last
requested extension:

```bash
openssl x509 \
  -in "$HOME/.step/camera/bosch-rtsps-chain.pem" \
  -noout -ext subjectAltName
```

Verify the path and IP identity:

```bash
openssl verify \
  -CAfile "$HOME/.step/certs/root_ca.crt" \
  -untrusted "$HOME/.step/certs/intermediate_ca.crt" \
  -verify_ip 10.0.30.11 \
  "$HOME/.step/camera/bosch-rtsps-chain.pem"
```

Confirm that the signed leaf uses the exact public key from the camera CSR:

```bash
csr_key=$(
  openssl req -in "$HOME/.step/camera/camera.csr.pem" -pubkey -noout |
    openssl pkey -pubin -outform DER 2>/dev/null |
    sha256sum | cut -d' ' -f1
)

cert_key=$(
  openssl x509 -in "$HOME/.step/camera/bosch-rtsps-chain.pem" -pubkey -noout |
    openssl pkey -pubin -outform DER 2>/dev/null |
    sha256sum | cut -d' ' -f1
)

test "$csr_key" = "$cert_key"
```

Do not upload a certificate when any of these checks fail.

## Upload and assign the certificate

1. Return to **Service > Certificates**.
2. Click **Add > Upload certificate**.
3. Select the leaf-plus-intermediate PEM chain.
4. Click **Upload** and wait for `100%`.
5. Confirm that the former CSR row is now a `Certificate`.
6. Confirm that the key icon is present. It proves that the camera associated
   the certificate with its retained private key.
7. Open the new certificate's **Usage** selector.
8. Select only **HTTPS**.
9. Leave **CBS client** assigned to the original Bosch `DeviceCertificate`.
10. Click **Set** and wait for the table to reload.

On the tested firmware, there is no separate SRTSP usage. RTSPS presents the
certificate assigned to HTTPS. Reassigning HTTPS therefore changes both the
web interface and RTSPS certificate.

After saving, the expected split is:

| Certificate | Usage |
| --- | --- |
| Private-CA camera certificate | `HTTPS` |
| Bosch `DeviceCertificate` | `CBS client` |

The browser may warn about the new HTTPS certificate until the private root CA
is trusted by the workstation.

## Account for the Bosch chain behavior

The tested firmware served only the leaf certificate on ports `443` and `9554`,
even when the uploaded file contained the leaf and intermediate. Uploading the
intermediate separately as a trusted camera certificate did not change the
served chain.

Confirm the behavior:

```bash
openssl s_client \
  -connect 10.0.30.11:9554 \
  -showcerts </dev/null 2>/dev/null |
  grep -c '^-----BEGIN CERTIFICATE-----$'
```

A result of `1` means the client must already have the issuing intermediate.
Create a portable trust bundle containing the intermediate and root:

```bash
step certificate bundle \
  "$HOME/.step/certs/intermediate_ca.crt" \
  "$HOME/.step/certs/root_ca.crt" \
  "$HOME/.step/camera/uug-camera-trust-bundle.pem"

chmod 644 "$HOME/.step/camera/uug-camera-trust-bundle.pem"
```

The trust bundle is public material. The CA private keys and password are not.

## Configure Kerberos Agent

For a process running directly in the same environment:

```dotenv
AGENT_CAPTURE_IPCAMERA_RTSP="rtsps://<user>:<password>@10.0.30.11:9554/?inst=1"
AGENT_CAPTURE_IPCAMERA_SUB_RTSP="rtsps://<user>:<password>@10.0.30.11:9554/?inst=2"
AGENT_CAPTURE_IPCAMERA_RTSPS_INSECURE=false
SSL_CERT_FILE=/home/agent/data/config/uug-camera-trust-bundle.pem
```

For a container, mount the public bundle read-only at the exact path visible
inside the container. The Agent image creates `/home/agent/data/config` and
includes Debian's `ca-certificates` package:

```bash
docker run \
  -v /secure/config/uug-camera-trust-bundle.pem:/home/agent/data/config/uug-camera-trust-bundle.pem:ro \
  -e SSL_CERT_FILE=/home/agent/data/config/uug-camera-trust-bundle.pem \
  -e AGENT_CAPTURE_IPCAMERA_RTSPS_INSECURE=false \
  -e 'AGENT_CAPTURE_IPCAMERA_RTSP=rtsps://<user>:<password>@10.0.30.11:9554/?inst=1' \
  -e 'AGENT_CAPTURE_IPCAMERA_SUB_RTSP=rtsps://<user>:<password>@10.0.30.11:9554/?inst=2' \
  kerberos/agent:latest
```

Restart the Agent after changing trust files. Go can cache the process system
certificate pool after its first use, so editing a file does not guarantee that
an already-running process reloads it.

The default production mode retains the image's normal public roots in addition
to the private camera CA. For a deliberately private-CA-only deployment, mount
an empty directory and set `SSL_CERT_DIR` to it:

```bash
-v /secure/config/empty-ca-dir:/home/agent/data/config/empty-ca-dir:ro \
-e SSL_CERT_DIR=/home/agent/data/config/empty-ca-dir
```

Only use that mode when the Agent does not need public roots for other TLS
connections.

## Validate the live endpoints

### Strict TLS and identity check

Use only the specified bundle, without OpenSSL's default CA locations:

```bash
openssl s_client \
  -brief \
  -connect 10.0.30.11:9554 \
  -verify_ip 10.0.30.11 \
  -verify_return_error \
  -CAfile "$HOME/.step/camera/uug-camera-trust-bundle.pem" \
  -no-CApath \
  -no-CAstore \
  </dev/null
```

Repeat with port `443`. Both must report `Verification: OK`.

Confirm that identity checking is active by repeating the command with a wrong
address, such as `-verify_ip 10.0.30.12`. It must fail with an IP address
mismatch.

### Confirm the live leaf is the generated leaf

```bash
live_fingerprint=$(
  openssl s_client -connect 10.0.30.11:9554 -showcerts </dev/null 2>/dev/null |
    openssl x509 -noout -fingerprint -sha256 |
    cut -d= -f2
)

local_fingerprint=$(
  openssl x509 \
    -in "$HOME/.step/camera/bosch-rtsps-chain.pem" \
    -noout -fingerprint -sha256 |
    cut -d= -f2
)

test "$live_fingerprint" = "$local_fingerprint"
```

### Validate the media path

A successful TLS handshake does not prove that RTSP authentication, DESCRIBE,
SETUP, PLAY, and RTP delivery work. Start a fresh Agent with verified TLS and
confirm that it connects without an x509 error and receives frames. During the
verified setup described here, a gortsplib probe completed all RTSP operations
and received an RTP packet over TCP.

## Why a tampered bundle may still connect

Editing PEM text is not always a useful negative TLS test.

### A certificate can still parse after a byte change

Base64 can remain syntactically valid when one character changes. OpenSSL may
still list the certificate subject and issuer even though a signature is now
invalid. Parsing and signature verification are different operations.

### Trust anchors are not validated through a parent

Every certificate loaded into Go's root pool is a trust anchor, including a
non-self-signed intermediate CA. Verification can terminate at that certificate.

If tampering changes only the intermediate's signature from its parent root,
but does not change its public key, that intermediate can still validate the
camera leaf when it is trusted directly. Its now-invalid parent signature is
not consulted at the trust boundary.

This is equivalent to OpenSSL's partial-chain behavior:

```bash
openssl s_client \
  -connect 10.0.30.11:9554 \
  -verify_ip 10.0.30.11 \
  -verify_return_error \
  -partial_chain \
  -CAfile tampered-bundle.pem \
  -no-CApath \
  -no-CAstore \
  </dev/null
```

### `SSL_CERT_FILE` does not isolate Go from CA directories

On Unix, Go uses `SSL_CERT_FILE` instead of its default aggregate CA file, but it
still scans default certificate directories such as `/etc/ssl/certs`. Setting
`SSL_CERT_FILE` alone therefore does not remove CA certificates installed with
`update-ca-certificates`.

Use exactly one trust-distribution approach when possible:

1. Mount a private trust bundle and set `SSL_CERT_FILE`; or
2. Install the CA certificates into the operating-system trust store.

Using both is valid, but makes isolation tests less obvious.

### Running processes can retain old roots

A long-running Go process may already have loaded and cached the trust pool.
Always start a new process after changing trust configuration during a negative
test.

## Perform a meaningful negative test

Do not corrupt only the root or intermediate signature. Instead, give a fresh
Agent process a completely unrelated CA and hide the default CA directories.

```bash
mkdir -p /tmp/empty-ca-dir

openssl req \
  -x509 -newkey rsa:2048 -nodes -days 1 \
  -subj '/CN=Unrelated Test Root' \
  -keyout /tmp/unrelated-test-root.key \
  -out /tmp/unrelated-test-root.crt

SSL_CERT_FILE=/tmp/unrelated-test-root.crt \
SSL_CERT_DIR=/tmp/empty-ca-dir \
AGENT_CAPTURE_IPCAMERA_RTSPS_INSECURE=false \
GOWORK=off \
go run -tags moq . -action run -port 8080
```

The connection must fail with an unknown-authority or chain-building error.
Delete the temporary test key and certificate afterward.

To test bundle integrity rather than client distrust, validate the intermediate
against the root explicitly:

```bash
openssl verify \
  -CAfile "$HOME/.step/certs/root_ca.crt" \
  -no-CApath \
  -no-CAstore \
  "$HOME/.step/certs/intermediate_ca.crt"
```

Store and compare approved SHA-256 fingerprints when detecting unauthorized
certificate-file changes is a requirement.

Restore an accidentally edited bundle from the protected CA certificates, then
restart the Agent:

```bash
step certificate bundle -f \
  "$HOME/.step/certs/intermediate_ca.crt" \
  "$HOME/.step/certs/root_ca.crt" \
  "$HOME/.step/camera/uug-camera-trust-bundle.pem"

openssl verify \
  -CAfile "$HOME/.step/certs/root_ca.crt" \
  -no-CApath \
  -no-CAstore \
  "$HOME/.step/certs/intermediate_ca.crt"
```

## Optional system trust installation

On Debian, install both public CA certificates when every process in the system
should trust this camera PKI:

```bash
sudo install -m 0644 \
  "$HOME/.step/certs/root_ca.crt" \
  /usr/local/share/ca-certificates/uug-camera-ca.crt

sudo install -m 0644 \
  "$HOME/.step/certs/intermediate_ca.crt" \
  /usr/local/share/ca-certificates/uug-camera-intermediate-ca.crt

sudo update-ca-certificates
```

This creates links below `/etc/ssl/certs`. Remove those files and rerun
`update-ca-certificates` before attempting an isolated trust-bundle test.

## Renewal and recovery

- Renew before the leaf or intermediate expires.
- Generate a new camera CSR if the firmware cannot renew the existing key.
- Sign the new CSR with all required SANs.
- Upload and validate the new certificate before deleting the old one.
- Preserve an alternate administrative access path while changing HTTPS usage.
- Back up the CA certificates, encrypted CA keys, and password separately.
- If the CA private keys are lost, create a new CA and redistribute its trust
  before replacing camera certificates.

## Production checklist

- [ ] The Agent URL uses `rtsps://`, not `rtsp://` or `srtsp://`.
- [ ] RTSPS is enabled on the camera and the configured port is reachable.
- [ ] The certificate SAN exactly matches the Agent URL host.
- [ ] The leaf public key matches the camera-generated CSR.
- [ ] The certificate has `serverAuth` extended key usage.
- [ ] The certificate expires before its issuer.
- [ ] HTTPS is assigned to the private-CA certificate.
- [ ] CBS client remains assigned to the Bosch device certificate.
- [ ] The Agent has the intermediate and root CA certificates it needs.
- [ ] `AGENT_CAPTURE_IPCAMERA_RTSPS_INSECURE=false`.
- [ ] The Agent was restarted after trust changes.
- [ ] A strict TLS check reports `Verification: OK`.
- [ ] A real Agent connection receives RTP packets.
- [ ] CA private keys and passwords are backed up outside the devcontainer.