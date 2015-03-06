package sysvipc

import (
	"syscall"
	"testing"
)

func TestFtok(t *testing.T) {
	for _, path := range []string{"common.go", "shm.go", "msg.go", "sem.go"} {
		key, err := Ftok(path, '+')
		if err != nil {
			t.Fatal(err)
		}

		key2, err := Ftok(path, '+')
		if err != nil {
			t.Fatal(err)
		}

		if key != key2 {
			t.Errorf("inconsistent result for %s and '+'", path)
		}
	}

	if _, err := Ftok("sem.go", 0); err == nil {
		t.Error("should have failed with a 0 projID")
	}

	if _, err := Ftok("missing_file", '+'); err != syscall.ENOENT {
		t.Error("should have failed ENOENT for missing path", err)
	}
}
