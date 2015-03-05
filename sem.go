package sysvipc

/*
#include <sys/types.h>
#include <sys/ipc.h>
#include <sys/sem.h>
int semget(key_t key, int nsems, int semflg);
int semtimedop(int semid, struct sembuf *sops, size_t nsops, const struct timespec *timeout);

union semctl_arg4 {
	int             val;
	struct semid_ds *buf;
	unsigned short  *array;
};
int semctl_buf(int semid, int cmd, struct semid_ds *buf) {
	union semctl_arg4 arg;
	arg.buf = buf;
	return semctl(semid, 0, cmd, arg);
};
int semctl_rm(int semid) {
	return semctl(semid, 0, IPC_RMID);
};
*/
import "C"
import (
	"errors"
	"time"
)

// SemaphoreSet is a kernel-maintained collection of semaphores.
type SemaphoreSet int64

// GetSemSet creates or retrieves the semaphore set for a given IPC key.
func GetSemSet(key, count, flag int64) (SemaphoreSet, error) {
	rc, err := C.semget(C.key_t(key), C.int(count), C.int(flag))
	if rc == -1 {
		return -1, err
	}
	return SemaphoreSet(rc), nil
}

// Run applies a group of SemOps atomically.
func (ss SemaphoreSet) Run(ops *SemOps, timeout time.Duration) error {
	var cto *C.struct_timespec
	if timeout >= 0 {
		sec := timeout / time.Second
		cto = &C.struct_timespec{
			tv_sec:  C.__time_t(sec),
			tv_nsec: C.__syscall_slong_t(timeout - sec),
		}
	}

	var opptr *C.struct_sembuf
	if len(*ops) > 0 {
		opptr = &(*ops)[0]
	}

	rc, err := C.semtimedop(C.int(ss), opptr, C.size_t(len(*ops)), cto)
	if rc == -1 {
		return err
	}
	return nil
}

// Stat produces information about the semaphore set.
func (ss SemaphoreSet) Stat() (*SemSetInfo, error) {
	sds := C.struct_semid_ds{}

	rc, err := C.semctl_buf(C.int(ss), C.IPC_STAT, &sds)
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
func (ss SemaphoreSet) Set(ssi *SemSetInfo) error {
	sds := &C.struct_semid_ds{
		sem_perm: C.struct_ipc_perm{
			uid:  C.__uid_t(ssi.Perms.OwnerUID),
			gid:  C.__gid_t(ssi.Perms.OwnerGID),
			mode: C.ushort(ssi.Perms.Mode & 0x1FF),
		},
	}

	rc, err := C.semctl_buf(C.int(ss), C.IPC_SET, sds)
	if rc == -1 {
		return err
	}
	return nil
}

// Remove deletes the semaphore set.
// This will also awake anyone blocked on the set with EIDRM.
func (ss SemaphoreSet) Remove() error {
	rc, err := C.semctl_rm(C.int(ss))
	if rc == -1 {
		return err
	}
	return nil
}

// SemOps is a collection of operations submitted to SemaphoreSet.Run.
type SemOps []C.struct_sembuf

// Increment adds an operation that will increase a semaphore's number.
func (so *SemOps) Increment(num uint16, by, flag int16) error {
	if by < 0 {
		return errors.New("sysvipc: by must be >0. use Decrement")
	} else if by == 0 {
		return errors.New("sysvipc: by must be >0. use WaitZero")
	}

	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op: C.short(by),
		sem_flg: C.short(flag),
	})
	return nil
}

// WaitZero adds and operation that will block until a semaphore's number is 0.
func (so *SemOps) WaitZero(num uint16, flag int16) error {
	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op: C.short(0),
		sem_flg: C.short(flag),
	})
	return nil
}

// Decrement adds an operation that will decrease a semaphore's number.
func (so *SemOps) Decrement(num uint16, by, flag int16) error {
	if by <= 0 {
		return errors.New("sysvipc: by must be >0. use WaitZero or Increment")
	}

	*so = append(*so, C.struct_sembuf{
		sem_num: C.ushort(num),
		sem_op: C.short(-by),
		sem_flg: C.short(flag),
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
