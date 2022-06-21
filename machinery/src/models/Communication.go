package models

import (
	"sync/atomic"
)

// The communication struct that is managing
// all the communication between the different goroutines.
type Communication struct {
	PackageCounter        *atomic.Value
	HandleBootstrap       chan string
	HandleStream          chan string
	HandleMotion          chan string
	HandleUpload          chan string
	HandleHeartBeat       chan string
	HandleLiveSD          chan int64
	HandleLiveHDKeepalive chan string
	HandleLiveHDHandshake chan SDPPayload
	HandleLiveHDPeers     chan string
	HandleONVIF           chan OnvifAction
}
