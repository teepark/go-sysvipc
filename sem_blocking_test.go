// +build linux

package sysvipc

import (
	"syscall"
	"testing"
	"time"
)

func TestSemBlockingDecrements(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	ops := NewSemOps()
	if err := ops.Decrement(0, 1, nil); err != nil {
		t.Fatal(err)
	}

	if err := ss.Run(ops, time.Millisecond); err != syscall.EAGAIN {
		t.Error("Decrement against 0 should have timed out", err)
	}
}
