package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/example/clipboard-sync/internal/protocol"
)

type client struct {
	connection net.Conn
	writeMu    sync.Mutex
}

type relay struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

func main() {
	listenAddress := flag.String("listen", ":65432", "TCP address to listen on")
	flag.Parse()

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatalf("listen on %s: %v", *listenAddress, err)
	}
	defer listener.Close()

	server := &relay{clients: make(map[*client]struct{})}
	log.Printf("TCP relay listening on %s", listener.Addr())
	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Printf("accept TCP connection: %v", err)
			continue
		}
		peer := &client{connection: connection}
		server.add(peer)
		log.Printf("client connected: %s", connection.RemoteAddr())
		go server.handle(peer)
	}
}

func (server *relay) add(peer *client) {
	server.mu.Lock()
	server.clients[peer] = struct{}{}
	server.mu.Unlock()
}

func (server *relay) remove(peer *client) {
	server.mu.Lock()
	delete(server.clients, peer)
	server.mu.Unlock()
	peer.connection.Close()
}

func (server *relay) handle(peer *client) {
	defer server.remove(peer)
	scanner := bufio.NewScanner(peer.connection)
	scanner.Buffer(make([]byte, 4096), protocol.MaxPacketSize)
	for scanner.Scan() {
		message, err := protocol.Parse(scanner.Bytes())
		if err != nil {
			log.Printf("discard invalid message from %s: %v", peer.connection.RemoteAddr(), err)
			continue
		}
		if err := server.broadcast(peer, message); err != nil {
			log.Printf("broadcast message from %s: %v", peer.connection.RemoteAddr(), err)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("read from %s: %v", peer.connection.RemoteAddr(), err)
	}
	log.Printf("client disconnected: %s", peer.connection.RemoteAddr())
}

func (server *relay) broadcast(sender *client, message protocol.Message) error {
	data, err := protocol.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	server.mu.Lock()
	clients := make([]*client, 0, len(server.clients))
	for peer := range server.clients {
		if peer != sender {
			clients = append(clients, peer)
		}
	}
	server.mu.Unlock()

	var firstError error
	for _, peer := range clients {
		peer.writeMu.Lock()
		peer.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
		err := writeAll(peer.connection, data)
		peer.connection.SetWriteDeadline(time.Time{})
		peer.writeMu.Unlock()
		if err != nil {
			if firstError == nil {
				firstError = fmt.Errorf("send to %s: %w", peer.connection.RemoteAddr(), err)
			}
			server.remove(peer)
			continue
		}
	}
	log.Printf("relayed clipboard update from %s (origin=%s, bytes=%d)", sender.connection.RemoteAddr(), message.Origin, len(data)-1)
	return firstError
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
