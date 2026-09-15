package packets

import "testing"

func TestLatestAtCurrentTailDoesNotSkipPacketWrittenAfterCreation(t *testing.T) {
	queue := NewQueue()
	defer queue.Close()
	cursor := queue.LatestAtCurrentTail()
	want := Packet{CurrentTime: 123, Data: []byte{1}}
	if err := queue.WritePacket(want); err != nil {
		t.Fatal(err)
	}
	got, err := cursor.ReadPacket()
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentTime != want.CurrentTime {
		t.Fatalf("packet timestamp = %d, want %d", got.CurrentTime, want.CurrentTime)
	}
}
