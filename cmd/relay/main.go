package main

import (
	"flag"
	"log"
	"net"

	"github.com/example/clipboard-sync/internal/protocol"
)

func main() {
	listenAddress := flag.String("listen", ":65432", "UDP address to listen on")
	broadcastAddress := flag.String("broadcast", "255.255.255.255:65432", "subnet broadcast destination")
	flag.Parse()

	listen, err := net.ResolveUDPAddr("udp", *listenAddress)
	if err != nil {
		log.Fatalf("resolve listen address: %v", err)
	}
	broadcast, err := net.ResolveUDPAddr("udp", *broadcastAddress)
	if err != nil {
		log.Fatalf("resolve broadcast address: %v", err)
	}
	listener, err := net.ListenUDP("udp", listen)
	if err != nil {
		log.Fatalf("listen on %s: %v", listen, err)
	}
	defer listener.Close()

	sender, err := net.DialUDP("udp", nil, broadcast)
	if err != nil {
		log.Fatalf("create broadcast socket: %v", err)
	}
	defer sender.Close()
	if err := enableBroadcast(sender); err != nil {
		log.Fatalf("enable UDP broadcast: %v", err)
	}

	log.Printf("UDP relay listening on %s and broadcasting to %s", listen, broadcast)
	buffer := make([]byte, protocol.MaxPacketSize+1)
	for {
		n, peer, err := listener.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("receive UDP packet: %v", err)
			continue
		}
		if peer.Port == sender.LocalAddr().(*net.UDPAddr).Port {
			continue
		}
		if _, err := protocol.Parse(buffer[:n]); err != nil {
			log.Printf("discard invalid packet from %s: %v", peer, err)
			continue
		}
		if _, err := sender.Write(buffer[:n]); err != nil {
			log.Printf("broadcast packet from %s: %v", peer, err)
		}
	}
}
