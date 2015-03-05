package sysvipc

/*
#include <string.h>
#include <sys/ipc.h>
#include <sys/shm.h>
int shmget(key_t key, size_t size, int shmflg);
void *shmat(int shmid, const void *shmaddr, int shmflg);
int shmdt(const void *shmaddr);
int shmctl(int shmid, int cmd, struct shmid_ds *buf);
*/
import "C"
import (
	"errors"
	"io"
	"time"
	"unsafe"
)

// SharedMem is an allocated block of memory sharable with multiple processes.
type SharedMem struct {
	id     int64
	length uint
}

// GetSharedMem creates or retrieves the shared memory segment for an IPC key
func GetSharedMem(key int64, size uint64, flag int64) (*SharedMem, error) {
	rc, err := C.shmget(C.key_t(key), C.size_t(size), C.int(flag))
	if rc == -1 {
		return nil, err
	}
	return &SharedMem{int64(rc), uint(size)}, nil
}

// Attach brings a shared memory segment into the current process's memory space.
func (shm *SharedMem) Attach(flag int64) (*SharedMemMount, error) {
	ptr, err := C.shmat(C.int(shm.id), nil, C.int(flag))
	if err != nil {
		return nil, err
	}

	return &SharedMemMount{ptr, 0, shm.length}, nil
}

// Stat produces meta information about the shared memory segment.
func (shm *SharedMem) Stat() (*SHMInfo, error) {
	shmds := C.struct_shmid_ds{}

	rc, err := C.shmctl(C.int(shm.id), C.IPC_STAT, &shmds)
	if rc == -1 {
		return nil, err
	}

	shminf := SHMInfo{
		Perms: IpcPerms{
			OwnerUID:   int(shmds.shm_perm.uid),
			OwnerGID:   int(shmds.shm_perm.gid),
			CreatorUID: int(shmds.shm_perm.cuid),
			CreatorGID: int(shmds.shm_perm.cgid),
			Mode:       uint16(shmds.shm_perm.mode),
		},
		SegmentSize:     uint(shmds.shm_segsz),
		LastAttach:      time.Unix(int64(shmds.shm_atime), 0),
		LastDetach:      time.Unix(int64(shmds.shm_dtime), 0),
		LastChange:      time.Unix(int64(shmds.shm_ctime), 0),
		CreatorPID:      int(shmds.shm_cpid),
		LastUserPID:     int(shmds.shm_lpid),
		CurrentAttaches: uint(shmds.shm_nattch),
	}

	return &shminf, nil
}

// Set updates parameters of the shared memory segment.
func (shm *SharedMem) Set(info *SHMInfo) error {
	shmds := &C.struct_shmid_ds{
		shm_perm: C.struct_ipc_perm{
			uid:  C.__uid_t(info.Perms.OwnerUID),
			gid:  C.__gid_t(info.Perms.OwnerGID),
			mode: C.ushort(info.Perms.Mode & 0x1FF),
		},
	}

	rc, err := C.shmctl(C.int(shm.id), C.IPC_SET, shmds)
	if rc == -1 {
		return err
	}
	return nil
}

// Remove marks the shared memory segment for removal.
// It will be removed when all attachments have been closed.
func (shm *SharedMem) Remove() error {
	rc, err := C.shmctl(C.int(shm.id), C.IPC_RMID, nil)
	if rc == -1 {
		return err
	}
	return nil
}

// SharedMemMount is the pointer to an attached block of shared memory space.
type SharedMemMount struct {
	ptr            unsafe.Pointer
	offset, length uint
}

// Read pulls bytes out of the shared memory segment.
func (shma *SharedMemMount) Read(p []byte) (int, error) {
	var err error
	l := uint(len(p))
	if l > (shma.length - shma.offset) {
		l = shma.length - shma.offset
		err = io.EOF
	}
	if l == 0 {
		return 0, err
	}

	src := unsafe.Pointer(uintptr(shma.ptr) + uintptr(shma.offset))
	dest := unsafe.Pointer(&p[0])

	C.memmove(dest, src, C.size_t(l))
	shma.offset += l
	return int(l), err
}

// Write places bytes into the shared memory segment.
func (shma *SharedMemMount) Write(p []byte) (int, error) {
	var err error
	l := uint(len(p))
	if l > (shma.length - shma.offset) {
		l = shma.length - shma.offset
		err = io.ErrShortWrite
	}
	if l == 0 {
		return 0, err
	}

	dest := unsafe.Pointer(uintptr(shma.ptr) + uintptr(shma.offset))
	src := unsafe.Pointer(&p[0])

	C.memmove(dest, src, C.size_t(l))
	shma.offset += l
	return int(l), err
}

// ReadByte returns a single byte from the current position in shared memory.
func (shma *SharedMemMount) ReadByte() (byte, error) {
	if shma.offset == shma.length {
		return 0, io.EOF
	}
	b := *(*byte)(unsafe.Pointer(uintptr(shma.ptr) + uintptr(shma.offset)))
	shma.offset++
	return b, nil
}

// UnreadByte sets the position back to before a ReadByte.
func (shma *SharedMemMount) UnreadByte() error {
	if shma.offset == 0 {
		return errors.New("sysvipc: UnreadByte before any ReadByte")
	}
	shma.offset--
	return nil
}

// WriteByte places a single byte at the current position in shared memory.
func (shma *SharedMemMount) WriteByte(c byte) error {
	if shma.offset == shma.length {
		return io.ErrShortWrite
	}
	b := (*byte)(unsafe.Pointer(uintptr(shma.ptr) + uintptr(shma.offset)))
	*b = c
	shma.offset++
	return nil
}

// Seek moves the current position in shared memory, according to "whence":
// - 0 makes the offset relative to the beginning
// - 1 makes it relative to the current position
// - 2 makes it relative to the end of the segment
func (shma *SharedMemMount) Seek(offset int64, whence int) (int64, error) {
	var endpos int64
	switch whence {
	case 0:
		endpos = offset
	case 1:
		endpos = int64(shma.offset) + offset
	case 2:
		endpos = int64(shma.length) + offset
	default:
		return 0, errors.New("sysvipc: bad 'whence' value")
	}

	if endpos < 0 {
		return int64(shma.offset), errors.New("sysvipc: negative offset")
	}

	if uint(endpos) > shma.length {
		shma.offset = shma.length
	} else {
		shma.offset = uint(endpos)
	}

	return int64(shma.offset), nil
}

// Close detaches the shared memory segment pointer.
func (shma *SharedMemMount) Close() error {
	rc, err := C.shmdt(shma.ptr)
	if rc == -1 {
		return err
	}
	return nil
}

// SHMInfo holds meta information about a shared memory segment.
type SHMInfo struct {
	Perms       IpcPerms
	SegmentSize uint

	LastAttach time.Time
	LastDetach time.Time
	LastChange time.Time

	CreatorPID  int
	LastUserPID int

	CurrentAttaches uint
}
