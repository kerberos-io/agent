import React, { useEffect, useRef } from 'react';
import './ClearKeyVideo.scss';

const DEFAULT_MIME = 'video/mp4; codecs="avc1.4d0033"';

const base64UrlToBytes = (value) => {
  if (!value) {
    return null;
  }
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
  try {
    const decoded = atob(padded);
    return Uint8Array.from(decoded, (c) => c.charCodeAt(0));
  } catch (err) {
    return null;
  }
};

const bytesToBase64Url = (bytes) => {
  const binary = Array.from(bytes, (b) => String.fromCharCode(b)).join('');
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
};

const hexToBytes = (hex) => {
  const cleaned = hex.toLowerCase();
  if (!/^[0-9a-f]+$/.test(cleaned) || cleaned.length % 2 !== 0) {
    return null;
  }
  const bytes = new Uint8Array(cleaned.length / 2);
  for (let i = 0; i < bytes.length; i += 1) {
    bytes[i] = parseInt(cleaned.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
};

const parseSymmetricKey = (value) => {
  if (!value) {
    return null;
  }
  const trimmed = value.trim();
  const noDashes = trimmed.replace(/-/g, '');

  if (noDashes.length === 32 && /^[0-9a-fA-F]+$/.test(noDashes)) {
    return hexToBytes(noDashes);
  }

  const base64Bytes = base64UrlToBytes(trimmed);
  if (base64Bytes && base64Bytes.length === 16) {
    return base64Bytes;
  }

  return null;
};

const sha256 = async (data) => {
  if (!window.crypto || !window.crypto.subtle) {
    return null;
  }
  const digest = await window.crypto.subtle.digest('SHA-256', data);
  return new Uint8Array(digest);
};

const deriveKeyMaterial = async (symmetricKey) => {
  const keyBytes = parseSymmetricKey(symmetricKey);
  if (keyBytes) {
    const sum = await sha256(keyBytes);
    if (!sum) {
      return null;
    }
    return {
      key: bytesToBase64Url(keyBytes),
      kid: bytesToBase64Url(sum.slice(0, 16)),
    };
  }

  const sum = await sha256(new TextEncoder().encode(symmetricKey));
  if (!sum) {
    return null;
  }
  return {
    key: bytesToBase64Url(sum.slice(0, 16)),
    kid: bytesToBase64Url(sum.slice(16, 32)),
  };
};

const readUint64 = (view, offset) => {
  const high = view.getUint32(offset);
  const low = view.getUint32(offset + 4);
  return high * 2 ** 32 + low;
};

const parseBoxes = (arrayBuffer) => {
  const view = new DataView(arrayBuffer);
  const boxes = [];
  let offset = 0;

  while (offset + 8 <= view.byteLength) {
    let size = view.getUint32(offset);
    const type = String.fromCharCode(
      view.getUint8(offset + 4),
      view.getUint8(offset + 5),
      view.getUint8(offset + 6),
      view.getUint8(offset + 7)
    );
    let headerSize = 8;
    if (size === 1) {
      if (offset + 16 > view.byteLength) {
        break;
      }
      size = readUint64(view, offset + 8);
      headerSize = 16;
    } else if (size === 0) {
      size = view.byteLength - offset;
    }

    if (size < headerSize || offset + size > view.byteLength) {
      break;
    }

    boxes.push({ type, start: offset, end: offset + size });
    offset += size;
  }

  return boxes;
};

const splitFmp4 = (arrayBuffer) => {
  const boxes = parseBoxes(arrayBuffer);
  const firstMediaIndex = boxes.findIndex(
    (box) => box.type === 'styp' || box.type === 'moof'
  );
  const initEnd = firstMediaIndex >= 0 ? boxes[firstMediaIndex].start : arrayBuffer.byteLength;
  const initSegment = arrayBuffer.slice(0, initEnd);

  const segments = [];
  let segmentStart = null;
  let lastMdatEnd = null;

  for (let i = firstMediaIndex; i < boxes.length; i += 1) {
    const box = boxes[i];
    if (!box) {
      continue;
    }

    if ((box.type === 'styp' || box.type === 'moof') && segmentStart === null) {
      segmentStart = box.start;
    }

    if (box.type === 'mdat') {
      lastMdatEnd = box.end;
    }

    const nextBox = boxes[i + 1];
    const nextStartsSegment = nextBox && (nextBox.type === 'styp' || nextBox.type === 'moof');
    if (segmentStart !== null && lastMdatEnd !== null && (nextStartsSegment || i === boxes.length - 1)) {
      segments.push(arrayBuffer.slice(segmentStart, lastMdatEnd));
      segmentStart = null;
      lastMdatEnd = null;
    }
  }

  return { initSegment, segments };
};

function ClearKeyVideo({
  src,
  mime = DEFAULT_MIME,
  kid = '',
  key = '',
  symmetricKey = '',
  className = '',
  controls = true,
}) {
  const videoRef = useRef(null);
  const hasKeyMaterial = !!symmetricKey || (!!kid && !!key);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !src || !hasKeyMaterial) {
      return undefined;
    }

    const mediaSource = new MediaSource();
    const objectUrl = URL.createObjectURL(mediaSource);
    video.src = objectUrl;

    let sourceBuffer;
    let pending = [];
    let endOfStream = false;
    let ended = false;
    let onUpdateEnd;

    const appendBuffer = (buffer) => {
      if (!sourceBuffer || !mediaSource || mediaSource.readyState !== 'open') {
        return;
      }
      if (sourceBuffer.updating || pending.length > 0) {
        pending.push(buffer);
        return;
      }
      sourceBuffer.appendBuffer(buffer);
    };

    let mediaKeysReadyResolve;
    const mediaKeysReady = new Promise((resolve) => {
      mediaKeysReadyResolve = resolve;
    });
    let mediaKeysSet = false;
    let keyMaterial = { kid, key };
    let keyMaterialPromise;

    const ensureKeyMaterial = async () => {
      if (kid && key) {
        return { kid, key };
      }
      if (!symmetricKey) {
        return keyMaterial;
      }
      if (!keyMaterialPromise) {
        keyMaterialPromise = deriveKeyMaterial(symmetricKey);
      }
      const derived = await keyMaterialPromise;
      if (derived) {
        keyMaterial = derived;
      }
      return keyMaterial;
    };

    const handleEncrypted = async (event) => {
      await mediaKeysReady;
      if (!video.mediaKeys) {
        return;
      }
      const resolved = await ensureKeyMaterial();
      if (!resolved.kid || !resolved.key) {
        return;
      }
      const session = video.mediaKeys.createSession();
      session.addEventListener('message', async () => {
        const resolved = await ensureKeyMaterial();
        if (!resolved.kid || !resolved.key) {
          return;
        }
        const license = JSON.stringify({
          keys: [{ kty: 'oct', kid: resolved.kid, k: resolved.key }],
          type: 'temporary',
        });
        const licenseBytes = new TextEncoder().encode(license);
        await session.update(licenseBytes);
      });
      await session.generateRequest(event.initDataType, event.initData);
    };

    const setupMediaKeys = async () => {
      if (mediaKeysSet) {
        return;
      }
      if (!MediaSource.isTypeSupported(mime)) {
        return;
      }
      const resolved = await ensureKeyMaterial();
      if (!resolved.kid || !resolved.key) {
        return;
      }
      const keyConfig = [
        {
          initDataTypes: ['cenc', 'keyids'],
          videoCapabilities: [{ contentType: mime }],
        },
      ];
      const access = await navigator.requestMediaKeySystemAccess(
        'org.w3.clearkey',
        keyConfig
      );
      const mediaKeys = await access.createMediaKeys();
      await video.setMediaKeys(mediaKeys);
      mediaKeysSet = true;
      mediaKeysReadyResolve();
    };

    video.addEventListener('encrypted', handleEncrypted);

    mediaSource.addEventListener('sourceopen', async () => {
      try {
        await setupMediaKeys();
        const response = await fetch(src);
        const arrayBuffer = await response.arrayBuffer();
        const { initSegment, segments } = splitFmp4(arrayBuffer);

        sourceBuffer = mediaSource.addSourceBuffer(mime);
        sourceBuffer.mode = 'segments';
        onUpdateEnd = () => {
          if (!sourceBuffer || mediaSource.readyState !== 'open') {
            return;
          }
          if (pending.length > 0) {
            const next = pending.shift();
            if (next) {
              sourceBuffer.appendBuffer(next);
            }
            return;
          }
          if (endOfStream && !ended) {
            ended = true;
            mediaSource.endOfStream();
          }
        };
        sourceBuffer.addEventListener('updateend', onUpdateEnd);

        appendBuffer(initSegment);
        segments.forEach((segment) => {
          appendBuffer(segment);
        });
        endOfStream = true;
        video.play().catch(() => {});
      } catch (err) {
        return;
      }
    });

    return () => {
      if (sourceBuffer && onUpdateEnd) {
        sourceBuffer.removeEventListener('updateend', onUpdateEnd);
      }
      video.removeEventListener('encrypted', handleEncrypted);
      if (video.mediaKeys) {
        video.pause();
        video.removeAttribute('src');
        video.load();
        video.setMediaKeys(null);
      }
      URL.revokeObjectURL(objectUrl);
    };
  }, [src, mime, kid, key, symmetricKey, hasKeyMaterial]);

  const videoClassName = `clearkey-video ${className}`.trim();
  if (!hasKeyMaterial) {
    return (
      <div className="clearkey-placeholder">
        Loading encryption key…
      </div>
    );
  }

  return <video ref={videoRef} className={videoClassName} controls={controls} />;
}

export default ClearKeyVideo;
