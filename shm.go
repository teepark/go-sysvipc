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
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	ErrReadOnlyShm = errors.New("Read-Only shared mem attachment")
)

// SharedMem is an allocated block of memory sharable with multiple processes.
type SharedMem struct {
	id     int64
	length uint
}

// GetSharedMem creates or retrieves the shared memory segment for an IPC key
func GetSharedMem(key int64, size uint64, flags *SHMFlags) (*SharedMem, error) {
	rc, err := C.shmget(C.key_t(key), C.size_t(size), C.int(flags.flags()))
	if rc == -1 {
		return nil, err
	}
	return &SharedMem{int64(rc), uint(size)}, nil
}

// Attach brings a shared memory segment into the current process's memory space.
func (shm *SharedMem) Attach(flags *SHMAttachFlags) (*SharedMemMount, error) {
	ptr, err := C.shmat(C.int(shm.id), nil, C.int(flags.flags()))
	if err != nil {
		return nil, err
	}

	return &SharedMemMount{ptr, 0, shm.length, flags.ro()}, nil
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
			mode: C.__mode_t(info.Perms.Mode & 0x1FF),
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

	// We have to store readonly here to prevent Write and WriteByte.
	// I'd be happy to let it panic, but C segfault panics can't recover.
	readonly bool
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

	memmove(dest, src, (uintptr)(l))
	shma.offset += l
	return int(l), err
}

// Write places bytes into the shared memory segment.
func (shma *SharedMemMount) Write(p []byte) (int, error) {
	if shma.readonly {
		// see comment on readonly field above
		return 0, ErrReadOnlyShm
	}

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

	memmove(dest, src, (uintptr)(l))
	shma.offset += l
	return int(l), err
}

// AtomicWriteUint32 places an uint32 value into the shared memory
// segment atomically (see "sync/atomic").
func (shma *SharedMemMount) AtomicWriteUint32(v uint32) error {
	if shma.readonly {
		// see comment on readonly field above
		return ErrReadOnlyShm
	}

	if (shma.length - shma.offset) < 4 {
		return io.ErrShortWrite
	}

	atomic.StoreUint32((*uint32)((unsafe.Pointer)(uintptr(shma.ptr)+uintptr(shma.offset))), v)
	shma.offset += 4
	return nil
}

// AtomicReadUint32 returns an uint32 value from the current position
// of the shared memory segment using atomic read (see "sync/atomic").
func (shma *SharedMemMount) AtomicReadUint32() (uint32, error) {
	if (shma.length - shma.offset) < 4 {
		return 0, io.EOF
	}

	v := atomic.LoadUint32((*uint32)((unsafe.Pointer)(uintptr(shma.ptr) + uintptr(shma.offset))))
	shma.offset += 4
	return v, nil
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
	if shma.readonly {
		// see comment on readonly field above
		return ErrReadOnlyShm
	}
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

// SHMFlags holds the options for GetSharedMem
type SHMFlags struct {
	// Create controls whether to create the shared memory segment if it
	// doesn't already exist.
	Create bool

	// Exclusive causes GetSharedMem to fail if the shared memory already
	// exists (only useful with Create).
	Exclusive bool

	// Perms is the file-style (rwxrwxrwx) permissions with which to create the
	// shared memory segment (also only useful with Create).
	Perms int
}

func (sf *SHMFlags) flags() int64 {
	if sf == nil {
		return 0
	}

	var f int64 = int64(sf.Perms) & 0777
	if sf.Create {
		f |= int64(C.IPC_CREAT)
	}
	if sf.Exclusive {
		f |= int64(C.IPC_EXCL)
	}

	return f
}

// SHMAttachFlags holds the options for SharedMem.Attach
type SHMAttachFlags struct {
	// ReadOnly causes the new SharedMemMount to be readable but not writable
	ReadOnly bool
}

func (sf *SHMAttachFlags) flags() int64 {
	if sf == nil {
		return 0
	}

	var f int64
	if sf.ReadOnly {
		f |= int64(C.SHM_RDONLY)
	}

	return f
}

func (sf *SHMAttachFlags) ro() bool {
	if sf == nil {
		return false
	}
	return sf.ReadOnly
}
