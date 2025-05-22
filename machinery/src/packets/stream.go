package packets

type Stream struct {
	// The ID of the stream.
	Index int

	// The name of the stream.
	Name string

	// The URL of the stream.
	URL string

	// Is the stream a video stream.
	IsVideo bool

	// Is the stream a audio stream.
	IsAudio bool

	// The width of the stream.
	Width int

	// The height of the stream.
	Height int

	// Num is the numerator of the framerate.
	Num int

	// Denum is the denominator of the framerate.
	Denum int

	// FPS is the framerate of the stream.
	FPS float64

	// For H264, this is the sps.
	SPS []byte

	// For H264, this is the pps.
	PPS []byte

	// For H265, this is the vps.
	VPS []byte

	// IsBackChannel is true if this stream is a back channel.
	IsBackChannel bool
}
