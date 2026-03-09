// AAC to G.711 µ-law transcoder using FFmpeg (libavcodec + libswresample).
// Build with:  go build -tags ffmpeg ...
//
// Requires:  libavcodec-dev, libavutil-dev, libswresample-dev  (FFmpeg ≥ 5.x)
// and an AAC decoder compiled into the FFmpeg build (usually the default).
//
//go:build ffmpeg

package webrtc

/*
#cgo pkg-config: libavcodec libavutil libswresample
#cgo CFLAGS: -Wno-deprecated-declarations

#include <libavcodec/avcodec.h>
#include <libavutil/channel_layout.h>
#include <libavutil/frame.h>
#include <libavutil/mem.h>
#include <libavutil/opt.h>
#include <libswresample/swresample.h>
#include <stdlib.h>
#include <string.h>

// ── Transcoder handle ───────────────────────────────────────────────────

typedef struct {
    AVCodecContext       *codec_ctx;
    AVCodecParserContext *parser;
    SwrContext           *swr_ctx;
    AVFrame              *frame;
    AVPacket             *pkt;
    int                   swr_initialized;
    int                   in_sample_rate;
    int                   in_channels;
} aac_transcoder_t;

// ── Create / Destroy ────────────────────────────────────────────────────

static aac_transcoder_t* aac_transcoder_create(void) {
    const AVCodec *codec = avcodec_find_decoder(AV_CODEC_ID_AAC);
    if (!codec) return NULL;

    aac_transcoder_t *t = (aac_transcoder_t*)calloc(1, sizeof(aac_transcoder_t));
    if (!t) return NULL;

    t->codec_ctx = avcodec_alloc_context3(codec);
    if (!t->codec_ctx) { free(t); return NULL; }

    if (avcodec_open2(t->codec_ctx, codec, NULL) < 0) {
        avcodec_free_context(&t->codec_ctx);
        free(t);
        return NULL;
    }

    t->parser = av_parser_init(AV_CODEC_ID_AAC);
    if (!t->parser) {
        avcodec_free_context(&t->codec_ctx);
        free(t);
        return NULL;
    }

    t->frame = av_frame_alloc();
    t->pkt   = av_packet_alloc();
    if (!t->frame || !t->pkt) {
        if (t->frame) av_frame_free(&t->frame);
        if (t->pkt)   av_packet_free(&t->pkt);
        av_parser_close(t->parser);
        avcodec_free_context(&t->codec_ctx);
        free(t);
        return NULL;
    }

    return t;
}

static void aac_transcoder_destroy(aac_transcoder_t *t) {
    if (!t) return;
    if (t->swr_ctx)   swr_free(&t->swr_ctx);
    if (t->frame)     av_frame_free(&t->frame);
    if (t->pkt)       av_packet_free(&t->pkt);
    if (t->parser)    av_parser_close(t->parser);
    if (t->codec_ctx) avcodec_free_context(&t->codec_ctx);
    free(t);
}

// ── Lazy resampler init (called after the first decoded frame) ──────────

static int aac_init_swr(aac_transcoder_t *t) {
    int64_t in_ch_layout = (int64_t)t->codec_ctx->channel_layout;
    if (in_ch_layout == 0)
        in_ch_layout = av_get_default_channel_layout(t->codec_ctx->channels);

    t->swr_ctx = swr_alloc_set_opts(
        NULL,
        AV_CH_LAYOUT_MONO,                             // out: mono
        AV_SAMPLE_FMT_S16,                             // out: signed 16-bit
        8000,                                           // out: 8 kHz
        in_ch_layout,                                   // in:  from decoder
        t->codec_ctx->sample_fmt,                       // in:  from decoder
        t->codec_ctx->sample_rate,                      // in:  from decoder
        0, NULL);

    if (!t->swr_ctx) return -1;
    if (swr_init(t->swr_ctx) < 0) {
        swr_free(&t->swr_ctx);
        return -1;
    }

    t->in_sample_rate = t->codec_ctx->sample_rate;
    t->in_channels    = t->codec_ctx->channels;
    t->swr_initialized = 1;
    return 0;
}

// ── Transcode ADTS → 8 kHz mono S16 PCM ────────────────────────────────
// Caller must free *out_pcm with av_free() when non-NULL.

static int aac_transcode_to_pcm(aac_transcoder_t *t,
                                 const uint8_t *data, int data_size,
                                 uint8_t **out_pcm, int *out_size) {
    *out_pcm  = NULL;
    *out_size = 0;
    if (!data || data_size <= 0) return 0;

    int buf_cap = 8192;
    uint8_t *buf = (uint8_t*)av_malloc(buf_cap);
    if (!buf) return -1;
    int buf_len = 0;

    while (data_size > 0) {
        uint8_t *pout = NULL;
        int      pout_size = 0;

        int used = av_parser_parse2(t->parser, t->codec_ctx,
                                    &pout, &pout_size,
                                    data, data_size,
                                    AV_NOPTS_VALUE, AV_NOPTS_VALUE, 0);
        if (used < 0) break;
        data      += used;
        data_size -= used;
        if (pout_size == 0) continue;

        // Feed parsed frame to decoder
        t->pkt->data = pout;
        t->pkt->size = pout_size;
        if (avcodec_send_packet(t->codec_ctx, t->pkt) < 0) continue;

        // Pull all decoded frames
        while (avcodec_receive_frame(t->codec_ctx, t->frame) == 0) {
            if (!t->swr_initialized) {
                if (aac_init_swr(t) < 0) {
                    av_frame_unref(t->frame);
                    av_free(buf);
                    return -1;
                }
            }

            int out_samples = swr_get_out_samples(t->swr_ctx,
                                                   t->frame->nb_samples);
            if (out_samples <= 0) out_samples = t->frame->nb_samples;

            int needed = buf_len + out_samples * 2; // S16 = 2 bytes/sample
            if (needed > buf_cap) {
                buf_cap = needed * 2;
                uint8_t *tmp = (uint8_t*)av_realloc(buf, buf_cap);
                if (!tmp) { av_frame_unref(t->frame); av_free(buf); return -1; }
                buf = tmp;
            }

            uint8_t *dst = buf + buf_len;
            int converted = swr_convert(t->swr_ctx,
                                        &dst, out_samples,
                                        (const uint8_t**)t->frame->extended_data,
                                        t->frame->nb_samples);
            if (converted > 0)
                buf_len += converted * 2;

            av_frame_unref(t->frame);
        }
    }

    if (buf_len == 0) {
        av_free(buf);
        return 0;
    }

    *out_pcm  = buf;
    *out_size = buf_len;
    return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/zaf/g711"
)

// AACTranscodingAvailable reports whether AAC→PCMU transcoding
// is compiled in (requires the "ffmpeg" build tag).
func AACTranscodingAvailable() bool { return true }

// AACTranscoder decodes ADTS-wrapped AAC audio to 8 kHz mono PCM
// and encodes it as G.711 µ-law for WebRTC transport.
type AACTranscoder struct {
	handle *C.aac_transcoder_t
}

// NewAACTranscoder creates a transcoder backed by FFmpeg's AAC decoder.
func NewAACTranscoder() (*AACTranscoder, error) {
	h := C.aac_transcoder_create()
	if h == nil {
		return nil, errors.New("failed to create AAC transcoder (FFmpeg AAC decoder not available?)")
	}
	log.Log.Info("webrtc.aac_transcoder: AAC → G.711 µ-law transcoder initialised (FFmpeg)")
	return &AACTranscoder{handle: h}, nil
}

// Transcode converts an ADTS buffer (one or more AAC frames) into
// G.711 µ-law encoded audio suitable for a PCMU WebRTC track.
func (t *AACTranscoder) Transcode(adtsData []byte) ([]byte, error) {
	if t == nil || t.handle == nil || len(adtsData) == 0 {
		return nil, nil
	}

	var outPCM *C.uint8_t
	var outSize C.int

	ret := C.aac_transcode_to_pcm(
		t.handle,
		(*C.uint8_t)(unsafe.Pointer(&adtsData[0])),
		C.int(len(adtsData)),
		&outPCM, &outSize,
	)
	if ret < 0 {
		return nil, errors.New("AAC decode/resample failed")
	}
	if outSize == 0 || outPCM == nil {
		return nil, nil // decoder buffering, no output yet
	}
	defer C.av_free(unsafe.Pointer(outPCM))

	// Copy S16LE PCM to Go slice, then encode to µ-law.
	pcm := C.GoBytes(unsafe.Pointer(outPCM), outSize)
	ulaw := g711.EncodeUlaw(pcm)

	// Log resampler details once.
	if t.handle.swr_initialized == 1 && t.handle.in_sample_rate != 0 {
		log.Log.Info(fmt.Sprintf(
			"webrtc.aac_transcoder: first output – resampling %d Hz / %d ch → 8000 Hz mono → µ-law",
			int(t.handle.in_sample_rate), int(t.handle.in_channels)))
		// Prevent repeated logging by zeroing the field we check.
		t.handle.in_sample_rate = 0
	}

	return ulaw, nil
}

// Close releases all FFmpeg resources held by the transcoder.
func (t *AACTranscoder) Close() {
	if t != nil && t.handle != nil {
		C.aac_transcoder_destroy(t.handle)
		t.handle = nil
		log.Log.Info("webrtc.aac_transcoder: transcoder closed")
	}
}
