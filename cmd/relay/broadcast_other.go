//go:build !linux

package main

import (
	"errors"
	"net"
)

func enableBroadcast(_ *net.UDPConn) error {
	return errors.New("the relay's broadcast socket is supported on Linux")
}
