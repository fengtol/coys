package protocol

import (
	"strings"
	"testing"
)

func TestMarshalParseRoundTrip(t *testing.T) {
	want := New("device-1", "你好, clipboard")
	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestRejectsOversizedPacketAndText(t *testing.T) {
	if _, err := Parse(make([]byte, MaxPacketSize+1)); err == nil {
		t.Fatal("Parse accepted an oversized packet")
	}
	if _, err := Marshal(New("device-1", strings.Repeat("x", MaxTextBytes+1))); err == nil {
		t.Fatal("Marshal accepted oversized text")
	}
}

func TestRejectsInvalidMessage(t *testing.T) {
	for _, message := range []Message{
		{Version: 2, Type: TypeClipboard, Origin: "device", Text: "text"},
		{Version: Version, Type: "other", Origin: "device", Text: "text"},
		{Version: Version, Type: TypeClipboard, Text: "text"},
		New("device", "before\x00after"),
	} {
		if err := Validate(message); err == nil {
			t.Errorf("Validate accepted %#v", message)
		}
	}
}
