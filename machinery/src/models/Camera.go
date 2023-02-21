package models

import "github.com/kerberos-io/joy4/av"

type Camera struct {
	Width     int
	Height    int
	Num       int
	Denum     int
	Framerate float64
	RTSP      string
	SubRTSP   string
	Codec     av.CodecType
}
