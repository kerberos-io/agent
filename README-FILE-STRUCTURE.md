# Recording file structure

This document describes where the Kerberos Agent stores recordings, locally and in
direct S3 uploads, and how the current layout relates to the legacy one.

## Local recording layout

New local recordings are stored under
`data/recordings/yyyy/mm/dd/<timestamp>-<deviceKey>.mp4`, using the recording's UTC
start date and Unix-millisecond timestamp. For example:
`data/recordings/2026/01/02/1767355200123-device-key.mp4`.
The filename no longer embeds the camera name, region, change count, or duration,
and it does not change when the recording is finalized.
Those attributes, together with FPS and encryption status, are captured in the
`.metadata` upload checkpoint and sent as structured HTTP/tus metadata.

Existing flat recordings and dated recordings with older long filenames remain
supported without a migration. Recording lists, day filters, playback, uploaders,
and automatic cleanup handle both filename formats and layouts. Upload queue
checkpoints remain flat in `data/cloud/`. Successful-upload removal also resolves
both local layouts. Deploy the updated Vault alongside Agent for canonical
`<account-directory>/yyyy/mm/dd/<timestamp>-<deviceKey>.mp4` storage keys and
metadata-driven Hub integration events.
Directory decryption also processes dated subdirectories and writes each output
to a sibling `decrypted/` directory; those outputs are excluded from recording
lists and automatic cleanup.

## Direct S3 recording layout

Direct S3 uploads store recordings as
`<s3.username>/yyyy/mm/dd/<timestamp>-<deviceKey>.mp4`. The timestamp is the
recording's Unix timestamp in milliseconds, and the date is its UTC date, not the upload
date. Timestamp and device key come from the recording's `.metadata` upload
checkpoint, which stores the timestamp in milliseconds. The uploader also
preserves checkpoint duration, FPS (when available), and encryption status as S3
object metadata. The device key must be non-empty and contain no path separators.
Retried uploads use the same object key, even after midnight. Millisecond
precision keeps recordings starting within the same second distinct.

This replaces the previous `<s3.username>/<filename>.mp4` layout for new uploads.
Existing objects are not moved; consumers of direct S3 uploads must support both
layouts when reading historical recordings. Updated Kerberos Vault deployments
use the same filename and dated layout under the authenticated account directory
for both direct Vault uploads and Hub-proxied uploads.

Only historical recordings with missing, empty, or FPS-only queue markers fall
back to the legacy filename format and configured Agent key. A structured
checkpoint takes precedence over both the filename and current configured key;
an invalid or unreadable checkpoint is reported as an upload error, not silently
replaced with filename-derived values.
For older structured checkpoints, missing camera name, region, and change count
are recovered from a validated legacy filename. Populated checkpoint fields
take precedence, and the checkpoint timestamp and device key remain authoritative.

The shorter filename cannot be decoded by legacy filename-only consumers.
Integrations must preserve structured recording metadata, including the device
key (`productid`) and recording timestamp (`event-timestamp`), as well as the full
object key.
