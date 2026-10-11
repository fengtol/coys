//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/example/clipboard-sync/internal/protocol"
)

type clipboardState struct {
	mu   sync.Mutex
	text string
}

func (state *clipboardState) syncLocal(
	connection *net.UDPConn,
	remote *net.UDPAddr,
	origin string,
) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	text, hasText, err := readClipboard()
	if err != nil {
		return err
	}
	if !hasText {
		return nil
	}
	if text == state.text {
		return nil
	}
	message, err := protocol.Marshal(protocol.New(origin, text))
	if err != nil {
		return err
	}
	if _, err := connection.WriteToUDP(message, remote); err != nil {
		return err
	}
	state.text = text
	return nil
}

func (state *clipboardState) applyRemote(text string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if text == state.text {
		return nil
	}
	if err := writeClipboard(text); err != nil {
		return err
	}
	state.text = text
	return nil
}

func main() {
	serverAddress := flag.String("server", "", "Debian relay IP address or hostname")
	serverPort := flag.Int("port", 65432, "relay UDP port")
	pollInterval := flag.Duration("poll", 500*time.Millisecond, "clipboard polling interval")
	flag.Parse()
	if *serverAddress == "" {
		log.Fatal("provide the relay address with -server")
	}
	if *serverPort < 1 || *serverPort > 65535 {
		log.Fatal("port must be between 1 and 65535")
	}
	if *pollInterval <= 0 {
		log.Fatal("poll interval must be positive")
	}

	remote, err := net.ResolveUDPAddr("udp", net.JoinHostPort(*serverAddress, fmt.Sprint(*serverPort)))
	if err != nil {
		log.Fatalf("resolve relay address: %v", err)
	}
	local, err := net.ResolveUDPAddr("udp", ":65432")
	if err != nil {
		log.Fatalf("resolve local address: %v", err)
	}
	connection, err := net.ListenUDP("udp", local)
	if err != nil {
		log.Fatalf("bind UDP port 65432: %v", err)
	}
	defer connection.Close()
	if err := enableBroadcast(connection); err != nil {
		log.Fatalf("enable UDP broadcast: %v", err)
	}

	origin, err := newOrigin()
	if err != nil {
		log.Fatalf("create client ID: %v", err)
	}
	state := &clipboardState{}
	if initialText, hasText, err := readClipboard(); err != nil {
		log.Fatalf("read initial clipboard: %v", err)
	} else if hasText {
		state.text = initialText
	} else {
		state.text = ""
	}

	log.Printf("clipboard client started; relay=%s; local UDP port=65432", remote)
	go receive(connection, origin, state)

	ticker := time.NewTicker(*pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := state.syncLocal(connection, remote, origin); err != nil {
			log.Printf("send clipboard update: %v", err)
		}
	}
}

func receive(connection *net.UDPConn, origin string, state *clipboardState) {
	buffer := make([]byte, protocol.MaxPacketSize+1)
	for {
		n, _, err := connection.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("receive UDP packet: %v", err)
			return
		}
		message, err := protocol.Parse(buffer[:n])
		if err != nil {
			log.Printf("discard invalid clipboard packet: %v", err)
			continue
		}
		if message.Origin == origin {
			continue
		}
		if err := state.applyRemote(message.Text); err != nil {
			log.Printf("write clipboard: %v", err)
		}
	}
}

func newOrigin() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func enableBroadcast(connection *net.UDPConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		socketErr = syscallSetsockoptBroadcast(fd)
	}); err != nil {
		return err
	}
	return socketErr
}
