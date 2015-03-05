package sysvipc

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSendRcv(t *testing.T) {
	setup(t)
	defer teardown(t)

	q.Send(6, []byte("test message body"), 0)
	msg, mtyp, err := q.Receive(64, -100, 0)
	if err != nil {
		t.Error(err)
	}

	if string(msg) != "test message body" || mtyp != 6 {
		t.Error(msg, mtyp)
	}
}

func TestStats(t *testing.T) {
	setup(t)
	defer teardown(t)

	msg := "this is a message in a test"

	info, err := q.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Perms.Mode != 0600 {
		t.Error("wrong permissions?")
	}
	if info.Perms.OwnerUID != os.Getuid() {
		t.Error("wrong owner?")
	}
	if info.Perms.CreatorUID != os.Getuid() {
		t.Error("wrong creator?")
	}
	if info.MsgCount != 0 {
		t.Error("phantom messages?")
	}

	before := time.Now()
	q.Send(4, []byte(msg), 0)
	after := time.Now()

	info, err = q.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.LastSend.After(after) {
		t.Error("timestamp late?")
	}
	if info.LastSend.Unix() < before.Unix() {
		// mq stat times are truncated to the second
		t.Error("timestamp early?")
	}
	if info.MsgCount != 1 {
		t.Error("missing message?")
	}
	if info.LastSender != os.Getpid() {
		t.Error("wrong last sender?")
	}
}

func TestSet(t *testing.T) {
	setup(t)
	defer teardown(t)

	info, err := q.Stat()
	if err != nil {
		t.Fatal(err)
	}

	mqi := &MQInfo{
		Perms: IpcPerms{
			OwnerUID: info.Perms.OwnerUID,
			OwnerGID: info.Perms.OwnerGID,
			Mode:     0644,
		},
	}

	err = q.Set(mqi)
	if err != nil {
		t.Fatal(err)
	}

	info, err = q.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Perms.Mode != 0644 {
		t.Error("perms change didn't take")
	}
}

func TestRemove(t *testing.T) {
	setup(t)
	defer teardown(t)

	if err := q.Remove(); err != nil {
		t.Fatal(err)
	}

	if _, err := q.Stat(); err != syscall.EINVAL {
		t.Fatal("stat on a removed queue should fail with EINVAL")
	}

	// so the teardown doesn't fail
	setup(t)
}

var q MessageQueue

func setup(t *testing.T) {
	mq, err := GetMsgQueue(0xDA7ABA5E, IPC_CREAT|IPC_EXCL|0600)
	if err != nil {
		t.Fatal(err)
	}
	q = mq
}

func teardown(t *testing.T) {
	if err := q.Remove(); err != nil {
		t.Fatal(err)
	}
}
