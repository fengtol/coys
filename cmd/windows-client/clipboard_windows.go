//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	openClipboard      = user32.NewProc("OpenClipboard")
	closeClipboard     = user32.NewProc("CloseClipboard")
	isClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	getClipboardData   = user32.NewProc("GetClipboardData")
	emptyClipboard     = user32.NewProc("EmptyClipboard")
	setClipboardData   = user32.NewProc("SetClipboardData")
	globalAlloc        = kernel32.NewProc("GlobalAlloc")
	globalLock         = kernel32.NewProc("GlobalLock")
	globalUnlock       = kernel32.NewProc("GlobalUnlock")
	globalFree         = kernel32.NewProc("GlobalFree")
	lengthWideString   = kernel32.NewProc("lstrlenW")
)

func readClipboard() (string, bool, error) {
	if ok, _, callErr := openClipboard.Call(0); ok == 0 {
		return "", false, fmt.Errorf("OpenClipboard: %w", callErr)
	}
	defer closeClipboard.Call()

	if available, _, _ := isClipboardFormatAvailable.Call(cfUnicodeText); available == 0 {
		return "", false, nil
	}
	handle, _, callErr := getClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", false, fmt.Errorf("GetClipboardData: %w", callErr)
	}
	pointer, _, callErr := globalLock.Call(handle)
	if pointer == 0 {
		return "", false, fmt.Errorf("GlobalLock: %w", callErr)
	}
	defer globalUnlock.Call(handle)

	length, _, callErr := lengthWideString.Call(pointer)
	if length > protocolMaxUTF16Units {
		return "", false, fmt.Errorf("clipboard text is longer than supported")
	}
	if length == 0 {
		return "", true, nil
	}
	value := unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), int(length))
	return syscall.UTF16ToString(value), true, nil
}

const protocolMaxUTF16Units = 48 * 1024

func writeClipboard(text string) error {
	wide, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("encode clipboard text: %w", err)
	}
	size := uintptr(len(wide)) * unsafe.Sizeof(wide[0])
	if ok, _, callErr := openClipboard.Call(0); ok == 0 {
		return fmt.Errorf("OpenClipboard: %w", callErr)
	}
	defer closeClipboard.Call()

	handle, _, callErr := globalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc: %w", callErr)
	}
	pointer, _, callErr := globalLock.Call(handle)
	if pointer == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("GlobalLock: %w", callErr)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), len(wide)), wide)
	globalUnlock.Call(handle)

	if ok, _, callErr := emptyClipboard.Call(); ok == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("EmptyClipboard: %w", callErr)
	}
	if result, _, callErr := setClipboardData.Call(cfUnicodeText, handle); result == 0 {
		globalFree.Call(handle)
		return fmt.Errorf("SetClipboardData: %w", callErr)
	}
	return nil
}
