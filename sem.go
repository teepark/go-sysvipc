package sysvipc

/*
#include <sys/types.h>
#include <sys/ipc.h>
#include <sys/sem.h>
int semget(key_t key, int nsems, int semflg);
int semtimedop(int semid, struct sembuf *sops, size_t nsops, const struct timespec *timeout);

union arg4 {
	int             val;
	struct semid_ds *buf;
	unsigned short  *array;
};
int semctl_noarg(int semid, int semnum, int cmd) {
	return semctl(semid, semnum, cmd);
};
int semctl_buf(int semid, int cmd, struct semid_ds *buf) {
	union arg4 arg;
	arg.buf = buf;
	return semctl(semid, 0, cmd, arg);
};
int semctl_arr(int semid, int cmd, unsigned short *arr) {
	union arg4 arg;
	arg.array = arr;
	return semctl(semid, 0, cmd, arg);
};
int semctl_val(int semid, int semnum, int cmd, int value) {
	union arg4 arg;
	arg.val = value;
	return semctl(semid, semnum, cmd, arg);
};
*/
import "C"
import (
	"errors"
	"time"
)

// SemaphoreSet is a kernel-maintained collection of semaphores.
type SemaphoreSet struct {
	id    int64
	count uint
}

// GetSemSet creates or retrieves the semaphore set for a given IPC key.
func GetSemSet(key, count int64, flags *SemSetFlags) (*SemaphoreSet, error) {
	rc, err := C.semget(C.key_t(key), C.int(count), C.int(flags.flags()))
	if rc == -1 {
		return nil, err
	}
	return &SemaphoreSet{int64(rc), uint(count)}, nil
}

// Run applies a group of SemOps atomically.
func (ss *SemaphoreSet) Run(ops *SemOps, timeout time.Duration) error {
	var cto *C.struct_timespec
	if timeout >= 0 {
		cto = &C.struct_timespec{
			tv_sec:  C.__time_t(timeout / time.Second),
			tv_nsec: C.__syscall_slong_t(timeout % time.Second),
		}
	}

	var opptr *C.struct_sembuf
	if len(*ops) > 0 {
		opptr = &(*ops)[0]
	}

	rc, err := C.semtimedop(C.int(ss.id), opptr, C.size_t(len(*ops)), cto)
	if rc == -1 {
		return err
	}
	return nil
}

// Getval retrieves the value of a single semaphore in the set
func (ss *SemaphoreSet) Getval(num uint16) (int, error) {
	val, err := C.semctl_noarg(C.int(ss.id), C.int(num), C.GETVAL)
	if val == -1 {
		return -1, err
	}
	return int(val), nil
}

// Setval sets the value of a single semaphore in the set
func (ss *SemaphoreSet) Setval(num uint16, value int) error {
	val, err := C.semctl_val(C.int(ss.id), C.int(num), C.SETVAL, C.int(value))
	if val == -1 {
		return err
	}
	return nil
}

// Getall retrieves the values of all the semaphores in the set
func (ss *SemaphoreSet) Getall() ([]uint16, error) {
	carr := make([]C.ushort, ss.count)

	rc, err := C.semctl_arr(C.int(ss.id), C.GETALL, &carr[0])
	if rc == -1 {
		return nil, err
	}

	results := make([]uint16, ss.count)
	for i, ci := range carr {
		results[i] = uint16(ci)
	}
	return results, nil
}

// Setall sets the values of every semaphore in the set
func (ss *SemaphoreSet) Setall(values []uint16) error {
	if uint(len(values)) != ss.count {
		return errors.New("sysvipc: wrong number of values for Setall")
	}

	carr := make([]C.ushort, ss.count)
	for i, val := range values {
		carr[i] = C.ushort(val)
	}

	rc, err := C.semctl_arr(C.int(ss.id), C.SETALL, &carr[0])
	if rc == -1 {
		return err
	}
	return nil
}

// Getpid returns the last process id to operate on the num-th semaphore
func (ss *SemaphoreSet) Getpid(num uint16) (int, error) {
	rc, err := C.semctl_noarg(C.int(ss.id), C.int(num), C.GETPID)
	if rc == -1 {
		return 0, err
	}
	return int(rc), nil
}

// GetNCnt returns the # of those blocked Decrementing the num-th semaphore
func (ss *SemaphoreSet) GetNCnt(num uint16) (int, error) {
	rc, err := C.semctl_noarg(C.int(ss.id), C.int(num), C.GETNCNT)
	if rc == -1 {
		return 0, err
	}
	return int(rc), nil
}

// GetZCnt returns the # of those blocked on WaitZero on the num-th semaphore
func (ss *SemaphoreSet) GetZCnt(num uint16) (int, error) {
	rc, err := C.semctl_noarg(C.int(ss.id), C.int(num), C.GETZCNT)
	if rc == -1 {
		return 0, err
	}
	return int(rc), nil
}

// Stat produces information about the semaphore set.
func (ss *SemaphoreSet) Stat() (*SemSetInfo, error) {
	sds := C.struct_semid_ds{}

	rc, err := C.semctl_buf(C.int(ss.id), C.IPC_STAT, &sds)
	if rc == -1 {
		return nil, err
	}

	ssinf := SemSetInfo{
		Perms: IpcPerms{
			OwnerUID:   int(sds.sem_perm.uid),
			OwnerGID:   int(sds.sem_perm.gid),
			CreatorUID: int(sds.sem_perm.cuid),
			CreatorGID: int(sds.sem_perm.cgid),
			Mode:       uint16(sds.sem_perm.mode),
		},
		LastOp:     time.Unix(int64(sds.sem_otime), 0),
		LastChange: time.Unix(int64(sds.sem_ctime), 0),
		Count:      uint(sds.sem_nsems),
	}
	return &ssinf, nil
}

// Set updates parameters of the semaphore set.
func (ss *SemaphoreSet) Set(ssi *SemSetInfo) error {
	sds := &C.struct_semid_ds{
		sem_perm: C.struct_ipc_perm{
			uid:  C.__uid_t(ssi.Perms.OwnerUID),
			gid:  C.__gid_t(ssi.Perms.OwnerGID),
			mode: C.__mode_t(ssi.Perms.Mode & 0x1FF),
		},
	}

	rc, err := C.semctl_buf(C.int(ss.id), C.IPC_SET, sds)
	if rc == -1 {
		return err
	}
	return nil
}

// Remove deletes the semaphore set.
// This will also awake anyone blocked on the set with EIDRM.
func (ss *SemaphoreSet) Remove() error {
	rc, err := C.semctl_noarg(C.int(ss.id), 0, C.IPC_RMID)
	if rc == -1 {
		return err
	}
	return nil
}

// SemOps is a collection of operations submitted to SemaphoreSet.Run.
type SemOps []C.struct_sembuf

func NewSemOps() *SemOps {
	sops := SemOps(make([]C.struct_sembuf, 0))
	return &sops
}

// Increment adds an operation that will increase a semaphore's number.
func (so *SemOps) Increment(num uint16, by int16, flags *SemOpFlags) error {
	if by < 0 {
		return errors.New("sysvipc: by must be >0. use Decrement")
	} else if by == 0 {
		return errors.New("sysvipc: by must be >0. use WaitZero")
	}

	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op:  C.short(by),
		sem_flg: C.short(flags.flags()),
	})
	return nil
}

// WaitZero adds and operation that will block until a semaphore's number is 0.
func (so *SemOps) WaitZero(num uint16, flags *SemOpFlags) error {
	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op:  C.short(0),
		sem_flg: C.short(flags.flags()),
	})
	return nil
}

// Decrement adds an operation that will decrease a semaphore's number.
func (so *SemOps) Decrement(num uint16, by int16, flags *SemOpFlags) error {
	if by <= 0 {
		return errors.New("sysvipc: by must be >0. use WaitZero or Increment")
	}

	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op:  C.short(-by),
		sem_flg: C.short(flags.flags()),
	})
	return nil
}

// SemSetInfo holds meta information about a semaphore set.
type SemSetInfo struct {
	Perms      IpcPerms
	LastOp     time.Time
	LastChange time.Time
	Count      uint
}

// SemSetFlags holds the options for a GetSemSet() call
type SemSetFlags struct {
	// Create controls whether to create the set if it doens't already exist.
	Create bool

	// Exclusive causes GetSemSet to fail if the semaphore set already exists
	// (only useful with Create).
	Exclusive bool

	// Perms is the file-style (rwxrwxrwx) permissions with which to create the
	// semaphore set (also only useful with Create).
	Perms int
}

func (sf *SemSetFlags) flags() int64 {
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

// SemOpFlags holds the options for SemOp methods
type SemOpFlags struct {
	// DontWait causes calls that would otherwise block
	// to instead fail with syscall.EAGAIN
	DontWait bool
}

func (so *SemOpFlags) flags() int64 {
	if so == nil {
		return 0
	}

	var f int64
	if so.DontWait {
		f |= int64(C.IPC_NOWAIT)
	}

	return f
}
