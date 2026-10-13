//go:build windows

package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/example/clipboard-sync/internal/protocol"
)

type readResult struct {
	message protocol.Message
	err     error
}

type clipboardState struct {
	text string
}

func (state *clipboardState) syncLocal(connection net.Conn, origin string) error {
	text, hasText, err := readClipboard()
	if err != nil {
		return err
	}
	if !hasText || text == state.text {
		return nil
	}
	message, err := protocol.Marshal(protocol.New(origin, text))
	if err != nil {
		return err
	}
	message = append(message, '\n')
	if err := connection.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if err := writeAll(connection, message); err != nil {
		return err
	}
	if err := connection.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}
	state.text = text
	return nil
}

func (state *clipboardState) applyRemote(text string) error {
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
	serverPort := flag.Int("port", 65432, "relay TCP port")
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

	origin, err := newOrigin()
	if err != nil {
		log.Fatalf("create client ID: %v", err)
	}
	state := &clipboardState{}
	if initialText, hasText, err := readClipboard(); err != nil {
		log.Fatalf("read initial clipboard: %v", err)
	} else if hasText {
		state.text = initialText
	}

	address := net.JoinHostPort(*serverAddress, fmt.Sprint(*serverPort))
	ticker := time.NewTicker(*pollInterval)
	defer ticker.Stop()
	for {
		connection, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			log.Printf("connect to relay %s: %v; retrying in 3 seconds", address, err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Printf("connected to TCP relay %s", address)
		incoming := make(chan readResult, 1)
		go receive(connection, incoming)
		connected := true
		for connected {
			select {
			case <-ticker.C:
				if err := state.syncLocal(connection, origin); err != nil {
					log.Printf("send clipboard update: %v", err)
					connected = false
				}
			case result := <-incoming:
				if result.err != nil {
					log.Printf("relay connection closed: %v", result.err)
					connected = false
					continue
				}
				if result.message.Origin == origin {
					continue
				}
				if err := state.applyRemote(result.message.Text); err != nil {
					log.Printf("write clipboard: %v", err)
				}
			}
		}
		connection.Close()
		time.Sleep(3 * time.Second)
	}
}

func receive(connection net.Conn, results chan<- readResult) {
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 4096), protocol.MaxPacketSize)
	for scanner.Scan() {
		message, err := protocol.Parse(scanner.Bytes())
		if err != nil {
			log.Printf("discard invalid relay message: %v", err)
			continue
		}
		results <- readResult{message: message}
	}
	err := scanner.Err()
	if err == nil {
		err = errors.New("connection reached EOF")
	}
	results <- readResult{err: err}
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func newOrigin() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}
