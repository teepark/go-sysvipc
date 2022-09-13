package sysvipc

/*
#include <sys/types.h>
#include <sys/ipc.h>
#include <sys/msg.h>
int msgget(key_t key, int msgflg);
int msgsnd(int msqid, const void *msgp, size_t msgsz, int msgflg);
ssize_t msgrcv(int msqid, void *msgp, size_t msgsz, long msgtyp, int msgflg);
int msgctl(int msqid, int cmd, struct msqid_ds *buf);
*/
import "C"
import (
	"time"
	"unsafe"
)

// MessageQueue is a kernel-maintained queue.
type MessageQueue int64

// GetMsgQueue creates or retrieves a message queue id for a given IPC key.
func GetMsgQueue(key int64, flags *MQFlags) (MessageQueue, error) {
	rc, err := C.msgget(C.key_t(key), C.int(flags.flags()))
	if rc == -1 {
		return -1, err
	}
	return MessageQueue(rc), nil
}

// Send places a new message onto the queue
func (mq MessageQueue) Send(mtyp int64, body []byte, flags *MQSendFlags) error {
	b := make([]byte, len(body)+8)
	copy(b[:8], serialize(mtyp))
	copy(b[8:], body)

	rc, err := C.msgsnd(
		C.int(mq),
		unsafe.Pointer(&b[0]),
		C.size_t(len(body)),
		C.int(flags.flags()),
	)
	if rc == -1 {
		return err
	}
	return nil
}

// Receive retrieves a message from the queue.
func (mq MessageQueue) Receive(maxlen uint, msgtyp int64, flags *MQRecvFlags) ([]byte, int64, error) {
	b := make([]byte, maxlen+8)

	rc, err := C.msgrcv(
		C.int(mq),
		unsafe.Pointer(&b[0]),
		C.size_t(maxlen),
		C.long(msgtyp),
		C.int(flags.flags()),
	)
	if rc == -1 {
		return nil, 0, err
	}

	mtyp := deserialize(b[:8])
	return b[8 : rc+8], mtyp, nil
}

// Stat produces information about the queue.
func (mq MessageQueue) Stat() (*MQInfo, error) {
	mqds := C.struct_msqid_ds{}

	rc, err := C.msgctl(C.int(mq), C.IPC_STAT, &mqds)
	if rc == -1 {
		return nil, err
	}

	mqinf := MQInfo{
		Perms: IpcPerms{
			OwnerUID:   int(mqds.msg_perm.uid),
			OwnerGID:   int(mqds.msg_perm.gid),
			CreatorUID: int(mqds.msg_perm.cuid),
			CreatorGID: int(mqds.msg_perm.cgid),
			Mode:       uint16(mqds.msg_perm.mode),
		},
		LastSend:   time.Unix(int64(mqds.msg_stime), 0),
		LastRcv:    time.Unix(int64(mqds.msg_rtime), 0),
		LastChange: time.Unix(int64(mqds.msg_ctime), 0),
		MsgCount:   uint(mqds.msg_qnum),
		MaxBytes:   uint(mqds.msg_qbytes),
		LastSender: int(mqds.msg_lspid),
		LastRcver:  int(mqds.msg_lrpid),
	}
	return &mqinf, nil
}

// Set updates parameters of the queue.
func (mq MessageQueue) Set(mqi *MQInfo) error {
	mqds := &C.struct_msqid_ds{
		msg_perm: C.struct_ipc_perm{
			uid:  C.__uid_t(mqi.Perms.OwnerUID),
			gid:  C.__gid_t(mqi.Perms.OwnerGID),
			mode: C.__mode_t(mqi.Perms.Mode & 0x1FF),
		},
		msg_qbytes: C.msglen_t(mqi.MaxBytes),
	}

	rc, err := C.msgctl(C.int(mq), C.IPC_SET, mqds)
	if rc == -1 {
		return err
	}
	return nil
}

// Remove deletes the queue.
// This will also awake all waiting readers and writers with EIDRM.
func (mq MessageQueue) Remove() error {
	rc, err := C.msgctl(C.int(mq), C.IPC_RMID, nil)
	if rc == -1 {
		return err
	}
	return nil
}

// MQInfo holds meta information about a message queue.
type MQInfo struct {
	Perms      IpcPerms
	LastSend   time.Time
	LastRcv    time.Time
	LastChange time.Time

	MsgCount uint
	MaxBytes uint

	LastSender int
	LastRcver  int
}

// MQFlags holds the flags/options for GetMsgQueue
type MQFlags struct {
	// Create controls whether to create the queue if it doesn't exist.
	Create bool

	// Exclusive causes GetMsgQueue to fail if the queue already exists (only
	// useful with Create).
	Exclusive bool

	// Perms is the file-style (rwxrwxrwx) permissions with which to create the
	// queue (also only useful with Create).
	Perms int
}

func (mf *MQFlags) flags() int64 {
	if mf == nil {
		return 0
	}

	var f int64 = int64(mf.Perms) & 0777
	if mf.Create {
		f |= int64(C.IPC_CREAT)
	}
	if mf.Exclusive {
		f |= int64(C.IPC_EXCL)
	}

	return f
}

// MQSendFlags hold the options for a MessageQueue.Send()
type MQSendFlags struct {
	// DontWait causes Send() calls that would otherwise
	// block to instead fail with syscall.EAGAIN
	DontWait bool
}

func (mf *MQSendFlags) flags() int64 {
	if mf == nil {
		return 0
	}

	var f int64
	if mf.DontWait {
		f |= int64(C.IPC_NOWAIT)
	}

	return f
}

// MQRecvFlags hold the options for a MessageQueue.Receive()
type MQRecvFlags struct {
	// DontWait causes Receive() calls that would otherwise
	// block to instead fail with syscall.EAGAIN or syscall.ENOMSG
	DontWait bool

	// Truncate allows shortening the message if maxlen is
	// shorter than the message being received
	Truncate bool
}

func (mf *MQRecvFlags) flags() int64 {
	if mf == nil {
		return 0
	}

	var f int64
	if mf.DontWait {
		f |= int64(C.IPC_NOWAIT)
	}
	if mf.Truncate {
		f |= int64(C.MSG_NOERROR)
	}

	return f
}

/*
real c-style pointer casting
*/

func serialize(num int64) []byte {
	b := make([]byte, 8)
	base := uintptr(unsafe.Pointer(&num))
	for i := 0; i < 8; i++ {
		b[i] = *(*byte)(unsafe.Pointer(base + uintptr(i)))
	}
	return b
}

func deserialize(b []byte) int64 {
	return *(*int64)(unsafe.Pointer(&b[0]))
}
