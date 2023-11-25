package packets

type Stream struct {
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

	// For H264, this is the sps.
	SPS []byte

	// For H264, this is the pps.
	PPS []byte
}
