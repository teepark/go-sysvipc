package sysvipc

import (
	"os"
	"testing"
)

var (
	shm   *SharedMem
	mount *SharedMemMount
)

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
}

func TestReadAndWriteByte(t *testing.T) {
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

	mnt2, err := shm.Attach(0)
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

	if inf.Perms.Mode & 0777 != 0644 {
		t.Error("mode change didn't take")
	}
}

func shmSetup(t *testing.T) {
	mem, err := GetSharedMem(0xDA7ABA5E, 4096, IPC_CREAT|IPC_EXCL|0600)
	if err != nil {
		t.Fatal(err)
	}
	shm = mem

	mnt, err := shm.Attach(0)
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
