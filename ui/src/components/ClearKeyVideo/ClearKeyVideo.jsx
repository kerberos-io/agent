import React, { useEffect, useRef } from 'react';
import './ClearKeyVideo.scss';

const DEFAULT_MIME = 'video/mp4; codecs="avc1.4d0033"';
const DEFAULT_KID = 'gqDkMcXXD6OswvkMod1mEA';
const DEFAULT_KEY = 'm3vJ57VuktuHrDz3tPv2ng';

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
  kid = DEFAULT_KID,
  key = DEFAULT_KEY,
  className = '',
  controls = true,
}) {
  const videoRef = useRef(null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !src) {
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

    const handleEncrypted = async (event) => {
      await mediaKeysReady;
      if (!video.mediaKeys) {
        return;
      }
      const session = video.mediaKeys.createSession();
      session.addEventListener('message', async () => {
        const license = JSON.stringify({
          keys: [{ kty: 'oct', kid, k: key }],
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
  }, [src, mime, kid, key]);

  const videoClassName = `clearkey-video ${className}`.trim();
  return <video ref={videoRef} className={videoClassName} controls={controls} />;
}

export default ClearKeyVideo;
