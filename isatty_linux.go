//go:build linux

package main

import (
	"syscall"
	"unsafe"
)

// isTerminal 用 ioctl(TCGETS) 判断 fd 是否为真正的终端（等价于 isatty）。
// 只判断 os.ModeCharDevice 会把 /dev/null 也当成终端，因此必须用 ioctl。
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
