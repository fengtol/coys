//go:build !windows

package main

import "log"

func main() {
	log.Fatal("the clipboard client is supported on Windows")
}
