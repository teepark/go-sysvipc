package sysvipc

import "testing"

func TestIncrements(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	target := []uint16{3, 2, 10, 4}

	ops := NewSemOps()
	for i, t := range target {
		ops.Increment(uint16(i), int16(t), 0)
	}

	if err := ss.Run(ops, -1); err != nil {
		t.Fatal(err)
	}

	vals, err := ss.Getall()
	if err != nil {
		t.Fatal(err)
	}

	for i, n := range target {
		if vals[i] != n {
			t.Error(i, vals[i], n)
		}
	}
}

var ss *SemaphoreSet

func semSetup(t *testing.T) {
	s, err := GetSemSet(0xDA7ABA5E, 4, IPC_CREAT|IPC_EXCL|0600)
	if err != nil {
		t.Fatal(err)
	}
	ss = s
}

func semTeardown(t *testing.T) {
	if err := ss.Remove(); err != nil {
		t.Error(err)
	}
}
