package sysvipc

import (
	"io"
	"os"
	"syscall"
	"testing"
)

func TestSHMErrors(t *testing.T) {
	if _, err := GetSharedMem(0xDA7ABA5E, 64, nil); err != syscall.ENOENT {
		t.Error("shmget without IPC_CREAT should have failed")
	}

	if _, err := (&SharedMem{5, 64}).Attach(nil); err != syscall.EINVAL && err != syscall.EIDRM {
		t.Error("shmat on a made-up shmid should fail", err)
	}

	if err := (&SharedMem{5, 64}).Remove(); err != syscall.EINVAL && err != syscall.EIDRM {
		t.Error("shmctl(IPC_RMID) on a made-up shmid should fail", err)
	}

	sm, err := GetSharedMem(0xDA7ABA5E, 64, &SHMFlags{
		Create:    true,
		Exclusive: true,
		Perms:     0600,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sm.Remove()
	mnt, err := sm.Attach(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mnt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mnt.Close(); err == nil {
		t.Error("double close should fail")
	}
}

func TestReadAndWrite(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	s := "this is a test string"

	_, err := mount.Write([]byte(s))
	if err != nil {
		t.Fatal(err)
	}

	_, err = mount.Seek(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	holder := make([]byte, len(s))
	_, err = mount.Read(holder)
	if err != nil {
		t.Error(err)
	}

	if string(holder) != s {
		t.Errorf("mismatched text, got back %v", holder)
	}

	_, err = mount.Seek(int64(-len(s)), 2)
	if err != nil {
		t.Fatal(err)
	}

	b := make([]byte, len(s)*2)
	i, err := mount.Read(b)
	if err != io.EOF {
		t.Error("a read that doesn't fill the buffer should give EOF", err)
	}
	if i != len(s) {
		t.Error("wrong length", i)
	}

	_, err = mount.Seek(0, 2)
	if err != nil {
		t.Fatal(err)
	}

	i, err = mount.Read(b)
	if err != io.EOF {
		t.Error("a read that comes up empty should give EOF", err)
	}
	if i != 0 {
		t.Error("wrong length", i)
	}

	i, err = mount.Write(b)
	if err != io.ErrShortWrite {
		t.Error("a write that couldn't complete should give ErrShortWrite", err)
	}
	if i != 0 {
		t.Error("wrong length", i)
	}

	_, err = mount.Seek(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = mount.AtomicWriteUint32(0x01020304)
	if err != nil {
		t.Fatal(err)
	}

	_, err = mount.Seek(0, 0)
	if err != nil {
		t.Fatal(err)
	}

	v, err := mount.AtomicReadUint32()
	if err != nil {
		t.Fatal(err)
	}

	if v != 0x01020304 {
		t.Errorf("Got %v, expected %v", v, 0x01020304)
	}
}

func TestSHMReadOnlyError(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	roat, err := shm.Attach(&SHMAttachFlags{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := roat.Write([]byte("ohai!")); err == nil {
		t.Error("should error on write to a read-only mount", err)
	}

	if err := roat.WriteByte('+'); err == nil {
		t.Error("should error on WriteByte to a read-only mount", err)
	}
}

func TestSHMSeeks(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	i, err := mount.Seek(2048, 0)
	if err != nil {
		t.Fatal(err)
	}
	if i != 2048 {
		t.Error("Seek to 2048 from the beginning should land at 2048", i)
	}

	j, err := mount.Seek(50, 1)
	if err != nil {
		t.Fatal(err)
	}
	if j != 2098 {
		t.Error("Seek forward 50 should have landed at 2098", j)
	}

	k, err := mount.Seek(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if k != 4096 {
		t.Error("Seek to end should land at 4096, the segment length", k)
	}

	_, err = mount.Seek(0, 3)
	if err == nil {
		t.Error("should fail on a bad 'whence'")
	}

	_, err = mount.Seek(-10, 0)
	if err == nil {
		t.Error("should fail when we end up at a negative index")
	}

	l, err := mount.Seek(7000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if l != 4096 {
		t.Error("should max out seeking to the end", l)
	}
}

func TestSHMReadAndWriteByte(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	s := "test string"
	for _, b := range []byte(s) {
		if err := mount.WriteByte(b); err != nil {
			t.Error(err)
		}
	}

	if _, err := mount.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	for i, b := range []byte(s) {
		c, err := mount.ReadByte()
		if err != nil {
			t.Error(err)
		}

		if b != c {
			t.Errorf("mismatched byte at position %d: %d vs %d", i, c, b)
		}
	}

	if _, err := mount.Seek(0, 2); err != nil {
		t.Fatal(err)
	}

	if _, err := mount.ReadByte(); err != io.EOF {
		t.Error("attempt to ReadByte from the end should produce EOF", err)
	}

	if err := mount.WriteByte('+'); err != io.ErrShortWrite {
		t.Error("attempt to WriteByte from the end should ErrShortWrite", err)
	}
}

func TestUnreadByte(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	s := "abcdefg"

	if _, err := mount.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}

	for i, c := range []byte(s) {
		if _, err := mount.Seek(int64(i), 0); err != nil {
			t.Fatal(err)
		}

		switch b, err := mount.ReadByte(); true {
		case err != nil:
			t.Error(err)
		case b != c:
			t.Error(i, c, b)
		}

		if err := mount.UnreadByte(); err != nil {
			t.Error(err)
		}

		switch b, err := mount.ReadByte(); true {
		case err != nil:
			t.Error(err)
		case b != c:
			t.Error(i, c, b)
		}
	}

	if _, err := mount.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	if err := mount.UnreadByte(); err == nil {
		t.Error("UnreadByte from beginning should fail", err)
	}
}

func TestSHMStat(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	info, err := shm.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Perms.Mode&0777 != 0600 {
		t.Error("wrong permissions", info.Perms.Mode)
	}
	if info.Perms.OwnerUID != os.Getuid() {
		t.Error("wrong owner")
	}
	if info.Perms.CreatorUID != os.Getuid() {
		t.Error("wrong creator")
	}
	if info.SegmentSize != 4096 {
		t.Error("wrong size:", info.SegmentSize)
	}
	if info.CreatorPID != os.Getpid() {
		t.Error("wrong creator pid")
	}
	if info.LastUserPID != os.Getpid() {
		t.Error("wrong last user pid")
	}
	if info.CurrentAttaches != 1 {
		t.Error("wrong number of attaches:", info.CurrentAttaches)
	}

	mnt2, err := shm.Attach(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mnt2.Close()

	info, err = shm.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.CurrentAttaches != 2 {
		t.Error("didn't get the extra attach?", info.CurrentAttaches)
	}
}

func TestSHMSet(t *testing.T) {
	shmSetup(t)
	defer shmTeardown(t)

	inf, err := shm.Stat()
	if err != nil {
		t.Fatal(err)
	}

	err = shm.Set(&SHMInfo{
		Perms: IpcPerms{
			OwnerUID: inf.Perms.OwnerUID,
			OwnerGID: inf.Perms.OwnerGID,
			Mode:     0644,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	inf, err = shm.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if inf.Perms.Mode&0777 != 0644 {
		t.Error("mode change didn't take")
	}
}

var (
	shm   *SharedMem
	mount *SharedMemMount
)

func shmSetup(t *testing.T) {
	mem, err := GetSharedMem(0xDA7ABA5E, 4096, &SHMFlags{
		Create:    true,
		Exclusive: true,
		Perms:     0600,
	})
	if err != nil {
		t.Fatal(err)
	}
	shm = mem

	mnt, err := shm.Attach(nil)
	if err != nil {
		t.Fatal(err)
	}
	mount = mnt

	err = shm.Remove()
	if err != nil {
		t.Fatal(err)
	}
}

func shmTeardown(t *testing.T) {
	mount.Close()
}
