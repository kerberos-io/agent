package rtsp

// mp4Muxer allows to save a H264 stream into a Mp4 file.
type mp4Muxer struct {
	sps []byte
	pps []byte
}

// newMp4Muxer allocates a mp4Muxer.
func newMp4Muxer(sps []byte, pps []byte) (*mp4Muxer, error) {
	return &mp4Muxer{
		sps: sps,
		pps: pps,
	}, nil
}
