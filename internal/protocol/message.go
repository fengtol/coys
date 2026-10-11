package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	Version       = 1
	TypeClipboard = "clipboard"
	MaxTextBytes  = 48 * 1024
	MaxPacketSize = 60 * 1024
)

type Message struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Origin  string `json:"origin"`
	Text    string `json:"text"`
}

func New(origin, text string) Message {
	return Message{Version: Version, Type: TypeClipboard, Origin: origin, Text: text}
}

func Marshal(message Message) ([]byte, error) {
	if err := Validate(message); err != nil {
		return nil, err
	}
	data, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal clipboard message: %w", err)
	}
	if len(data) > MaxPacketSize {
		return nil, fmt.Errorf("clipboard message exceeds %d bytes", MaxPacketSize)
	}
	return data, nil
}

func Parse(data []byte) (Message, error) {
	if len(data) == 0 || len(data) > MaxPacketSize {
		return Message{}, fmt.Errorf("packet size must be between 1 and %d bytes", MaxPacketSize)
	}
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return Message{}, fmt.Errorf("decode clipboard message: %w", err)
	}
	if err := Validate(message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func Validate(message Message) error {
	switch {
	case message.Version != Version:
		return fmt.Errorf("unsupported protocol version %d", message.Version)
	case message.Type != TypeClipboard:
		return fmt.Errorf("unsupported message type %q", message.Type)
	case message.Origin == "" || len(message.Origin) > 64:
		return errors.New("origin must contain 1 to 64 bytes")
	case !utf8.ValidString(message.Text):
		return errors.New("clipboard text is not valid UTF-8")
	case strings.ContainsRune(message.Text, '\x00'):
		return errors.New("clipboard text must not contain NUL")
	case len(message.Text) > MaxTextBytes:
		return fmt.Errorf("clipboard text exceeds %d bytes", MaxTextBytes)
	default:
		return nil
	}
}
