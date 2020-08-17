package sysvipc

import "unsafe"

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)
