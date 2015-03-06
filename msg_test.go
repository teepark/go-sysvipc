package sysvipc

import (
	"os"
	"syscall"
	"testing"
)

func TestMSGBadGet(t *testing.T) {
	if _, err := GetMsgQueue(0xDA7ABA5E, 0); err != syscall.ENOENT {
		t.Error("GetMsgQueue on a non-existent queue without CREAT should fail")
	}
}

func TestSendRcv(t *testing.T) {
	msgSetup(t)
	defer msgTeardown(t)

	if err := q.Send(-1, nil, 0); err != syscall.EINVAL {
		t.Error("msgsnd with negative mtyp should fail", err)
	}

	q.Send(6, []byte("test message body"), 0)
	msg, mtyp, err := q.Receive(64, -100, 0)
	if err != nil {
		t.Error(err)
	}

	if string(msg) != "test message body" || mtyp != 6 {
		t.Errorf("%q %v", string(msg), mtyp)
	}
}

func TestMSGStats(t *testing.T) {
	msgSetup(t)
	defer msgTeardown(t)

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

	q.Send(4, []byte(msg), 0)

	info, err = q.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.MsgCount != 1 {
		t.Error("missing message?")
	}
	if info.LastSender != os.Getpid() {
		t.Error("wrong last sender?")
	}
}

func TestMSGSet(t *testing.T) {
	msgSetup(t)
	defer msgTeardown(t)

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
	msgSetup(t)
	defer msgTeardown(t)

	if err := q.Remove(); err != nil {
		t.Fatal(err)
	}

	if _, err := q.Stat(); err != syscall.EINVAL {
		t.Fatal("stat on a removed queue should fail with EINVAL")
	}

	// so the msgTeardown doesn't fail
	msgSetup(t)
}

var q MessageQueue

func msgSetup(t *testing.T) {
	mq, err := GetMsgQueue(0xDA7ABA5E, IPC_CREAT|IPC_EXCL|0600)
	if err != nil {
		t.Fatal(err)
	}
	q = mq
}

func msgTeardown(t *testing.T) {
	if err := q.Remove(); err != nil {
		t.Fatal(err)
	}
}
