package components

import (
	"github.com/kerberos-io/joy4/av"
)

type RingBuffer struct {
	inputChannel  chan av.Packet
	outputChannel chan av.Packet
}

func NewRingBuffer(inputChannel chan av.Packet, outputChannel chan av.Packet) *RingBuffer {
	return &RingBuffer{inputChannel, outputChannel}
}

func (r *RingBuffer) Run() {
	for v := range r.inputChannel {
		select {
		case r.outputChannel <- v:
		default:
			select {
			case <-r.outputChannel:
				r.outputChannel <- v
			default:
			}
		}
	}
	close(r.outputChannel)
	for _ = range r.outputChannel {
	}
}

func (r *RingBuffer) Close() {
	close(r.inputChannel)
	for _ = range r.inputChannel {
	}
}

func CreateBuffer(bufferSize int) (*RingBuffer, chan av.Packet, chan av.Packet) {
	write := make(chan av.Packet)
	read := make(chan av.Packet, bufferSize)
	buffer := NewRingBuffer(write, read)
	go buffer.Run()
	return buffer, write, read
}
