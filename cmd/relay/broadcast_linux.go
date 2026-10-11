//go:build linux

package main

import (
	"fmt"
	"net"
	"syscall"
)

func enableBroadcast(connection *net.UDPConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return fmt.Errorf("get UDP socket: %w", err)
	}
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		socketErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		return fmt.Errorf("configure UDP socket: %w", err)
	}
	if socketErr != nil {
		return fmt.Errorf("enable SO_BROADCAST: %w", socketErr)
	}
	return nil
}
