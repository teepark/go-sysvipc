package sysvipc

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestSemBadGet(t *testing.T) {
	// no CREAT, doesn't exist
	semset, err := GetSemSet(0xDA7ABA5E, 3, nil)
	if err != syscall.ENOENT {
		t.Error(err)
	} else if err == nil {
		semset.Remove()
	}

	// 0 count
	semset, err = GetSemSet(0xDA7ABA5E, 0, &SemSetFlags{Create: true})
	if err != syscall.EINVAL {
		t.Error(err)
	} else if err == nil {
		semset.Remove()
	}
}

func TestSemBadRemove(t *testing.T) {
	s := &SemaphoreSet{5, 2} // 5 was never created
	if err := s.Remove(); err != syscall.EIDRM {
		t.Fatal(err)
	}
}

func TestSemIncrements(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	target := []uint16{3, 2, 10, 4}

	ops := NewSemOps()
	for i, t := range target {
		ops.Increment(uint16(i), int16(t), nil)
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

	if err := ops.Increment(0, -1, &SemOpFlags{DontWait: true}); err == nil {
		t.Error("negative increment should fail")
	}
	if err := ops.Increment(0, 0, nil); err == nil {
		t.Error("zero increment should fail")
	}
}

func TestSemDecrements(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	if err := ss.Setval(0, 5); err != nil {
		t.Fatal(err)
	}

	ops := NewSemOps()
	ops.Decrement(0, 2, nil)
	if err := ss.Run(ops, -1); err != nil {
		t.Fatal(err)
	}

	val, err := ss.Getval(0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 3 {
		t.Error("decrement didn't take")
	}

	ops = NewSemOps()
	ops.Decrement(0, 1, nil)
	if err := ss.Run(ops, -1); err != nil {
		t.Fatal(err)
	}

	val, err = ss.Getval(0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Error("decrement didn't take")
	}

	if err := ops.Decrement(0, -1, nil); err == nil {
		t.Error("negative decrement should fail")
	}
	if err := ops.Decrement(0, 0, nil); err == nil {
		t.Error("zero decrement should fail")
	}
}

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

func TestSemNonBlockingDecrements(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	ops := NewSemOps()
	if err := ops.Decrement(0, 1, &SemOpFlags{DontWait: true}); err != nil {
		t.Fatal(err)
	}

	// I'd love to skip the separate goroutine and do this with Run's timeout,
	// but that would also fail with EAGAIN and I wouldn't know which
	// (IPC_NOWAIT or the timeout) caused it.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := ss.Run(ops, -1); err != syscall.EAGAIN {
			t.Error("non-blocking decrement against 0 should fail", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		t.Error("timeout passed")
	}
}

func TestSemWaitZero(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	if err := ss.Setval(0, 3); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ops := NewSemOps()
		if err := ops.WaitZero(0, nil); err != nil {
			t.Fatal(err)
		}
		if err := ss.Run(ops, -1); err != nil {
			t.Error(err)
		}
	}()

	runtime.Gosched()

	select {
	case <-done:
		t.Error("WaitZero returned before setting sem to 0")
	default:
	}

	if err := ss.Setval(0, 0); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	<-done
	if elapsed := time.Since(start); elapsed > 1*time.Millisecond {
		t.Error("WaitZero didn't unblock fast enough:", elapsed)
	}
}

func TestSemWaitZeroTimeout(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	if err := ss.Setval(0, 1); err != nil {
		t.Fatal(err)
	}

	ops := NewSemOps()
	if err := ops.WaitZero(0, nil); err != nil {
		t.Fatal(err)
	}

	if err := ss.Run(ops, 1*time.Millisecond); err != syscall.EAGAIN {
		t.Fatal(err)
	}
}

func TestSemWaitZeroNonBlocking(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	if err := ss.Setval(0, 1); err != nil {
		t.Fatal(err)
	}

	ops := NewSemOps()
	if err := ops.WaitZero(0, &SemOpFlags{DontWait: true}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := ss.Run(ops, -1); err != syscall.EAGAIN {
			t.Error("waitzero non-blocking should fail", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		t.Error("timed out")
	}
}

func TestSemSetAndGetVals(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	vals := []int{4, 3, 9, 6}

	for i, val := range vals {
		if err := ss.Setval(uint16(i), val); err != nil {
			t.Fatal(err)
		}
	}

	for i, val := range vals {
		stored, err := ss.Getval(uint16(i))
		if err != nil {
			t.Fatal(err)
		}

		if val != stored {
			t.Error("mismatched values:", val, stored)
		}
	}

	// test failure on a negative number
	if err := ss.Setval(0, -1); err != syscall.ERANGE {
		t.Fatal(err)
	}
}

func TestSemGetValNotAllowed(t *testing.T) {
	s, err := GetSemSet(0xDA7ABA5E, 1, &SemSetFlags{
		Create:    true,
		Exclusive: true,
		Perms:     0, // no read perms, even for owner
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Remove(); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := s.Getval(0); err != syscall.EACCES {
		t.Error(err)
	}
}

func TestSemSetAndGetAll(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	target := []uint16{4, 5, 6, 7}

	if err := ss.Setall(target[:3]); err == nil {
		t.Error("Setall should have failed when given too few values")
	}

	if err := ss.Setall(target); err != nil {
		t.Fatal(err)
	}

	got, err := ss.Getall()
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(target) {
		t.Error("didn't get back what we stored in semset")
	} else {
		for i := range target {
			if got[i] != target[i] {
				t.Error("didn't get back what we stored in semset")
			}
		}
	}
}

func TestSemStat(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	// EIDRM with a bad semset id
	if _, err := (&SemaphoreSet{5, 2}).Stat(); err != syscall.EIDRM {
		t.Error("semctl(IPC_STAT) on a made up semset id should fail")
	}

	info, err := ss.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Perms.OwnerUID != os.Getuid() {
		t.Error("wrong owner uid", info.Perms.OwnerUID)
	}
	if info.Perms.CreatorUID != os.Getuid() {
		t.Error("wrong creator uid", info.Perms.CreatorUID)
	}
	if info.Perms.Mode&0777 != 0600 {
		t.Error("wrong mode", info.Perms.Mode)
	}
	if info.Count != 4 {
		t.Error("wrong count", info.Count)
	}
}

func TestSemSetSet(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	info, err := ss.Stat()
	if err != nil {
		t.Fatal(err)
	}

	set := &SemSetInfo{
		Perms: IpcPerms{
			OwnerUID: info.Perms.OwnerUID,
			OwnerGID: info.Perms.OwnerGID,
			Mode:     0400,
		},
	}
	if err := ss.Set(set); err != nil {
		t.Fatal(err)
	}

	info, err = ss.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Perms.Mode&0777 != 0400 {
		t.Error("set() didn't take")
	}
}

func TestSemGetpid(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	ops := NewSemOps()
	if err := ops.Increment(0, 1, nil); err != nil {
		t.Fatal(err)
	}
	if err := ss.Run(ops, -1); err != nil {
		t.Fatal(err)
	}

	pid, err := ss.Getpid(0)
	if err != nil {
		t.Fatal(err)
	}

	if pid != os.Getpid() {
		t.Error("we should be the last pid to operate on sem 0")
	}

	if _, err := ss.Getpid(7); err != syscall.EINVAL {
		t.Error("Getpid should fail with EINVAL for an out-of-bounds num", err)
	}
}

func TestSemGetNCnt(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	cnt, err := ss.GetNCnt(0)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Error("shouldn't be any decrement waiters yet")
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			ops := NewSemOps()
			if err := ops.Decrement(0, 1, nil); err != nil {
				t.Fatal(err)
			}
			wg.Done()
			ss.Run(ops, -1)
		}()
	}

	wg.Wait()
	runtime.Gosched()

	cnt, err = ss.GetNCnt(0)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 3 {
		t.Error("should be 3 waiters, instead have", cnt)
	}

	cnt, err = ss.GetNCnt(14)
	if err != syscall.EINVAL {
		t.Error("GetNCnt with out-of-bounds num should fail")
	}
}

func TestSemGetZCnt(t *testing.T) {
	semSetup(t)
	defer semTeardown(t)

	if err := ss.Setval(0, 2); err != nil {
		t.Fatal(err)
	}

	cnt, err := ss.GetZCnt(0)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Error("no waiters, GetZCnt should be 0")
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			ops := NewSemOps()
			if err := ops.WaitZero(0, nil); err != nil {
				t.Fatal(err)
			}

			wg.Done()

			ss.Run(ops, time.Millisecond)
		}()
	}

	wg.Wait()
	runtime.Gosched()

	cnt, err = ss.GetZCnt(0)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 3 {
		t.Error("should be 3 waiters, instead have", cnt)
	}

	_, err = ss.GetZCnt(11)
	if err != syscall.EINVAL {
		t.Error("GetZCnt should fail with an out-of-bounds num")
	}
}

func TestSemBadSet(t *testing.T) {
	if _, err := (&SemaphoreSet{5, 2}).Stat(); err != syscall.EIDRM {
		t.Error("semctl(IPC_SET) on a made up semset id should fail")
	}
}

var ss *SemaphoreSet

func semSetup(t *testing.T) {
	s, err := GetSemSet(0xDA7ABA5E, 4, &SemSetFlags{
		Create:    true,
		Exclusive: true,
		Perms:     0600,
	})
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
