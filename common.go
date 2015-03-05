package sysvipc

/*
#include <stdlib.h>
#include <sys/types.h>
#include <sys/ipc.h>
#include <sys/msg.h>
#include <sys/shm.h>
key_t ftok(const char *pathname, int proj_id);
*/
import "C"
import (
	"errors"
	"unsafe"
)

// Flag constants
const (
	IPC_CREAT   = C.IPC_CREAT
	IPC_EXCL    = C.IPC_EXCL
	IPC_NOWAIT  = C.IPC_NOWAIT
	MSG_NOERROR = C.MSG_NOERROR
	SHM_RDONLY  = C.SHM_RDONLY
	SHM_RND     = C.SHM_RND
)

// IpcPerms holds information about the permissions of a SysV IPC object.
type IpcPerms struct {
	OwnerUID   int
	OwnerGID   int
	CreatorUID int
	CreatorGID int
	Mode       uint16
}

// Ftok creates a System V IPC key suitable for msgget, semget, or shmget.
// pathname must be an existing, accessible file, and projID must not be 0.
func Ftok(pathname string, projID uint8) (int64, error) {
	if projID == 0 {
		return -1, errors.New("sysvipc: projID must be nonzero")
	}

	cpath := C.CString(pathname)
	defer C.free(unsafe.Pointer(cpath))

	rckey, err := C.ftok(cpath, C.int(projID))
	rc := int64(rckey)
	if rc == -1 {
		return -1, err
	}

	return rc, nil
}
