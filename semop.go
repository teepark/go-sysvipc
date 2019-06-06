// +build !linux

package sysvipc

/*
#include <sys/types.h>
#include <sys/ipc.h>
#include <sys/sem.h>

int semop(int semid, struct sembuf *sops, size_t nsops);
*/
import "C"

func semtimedop(semid C.int, sops *C.struct_sembuf, nsops C.size_t, timeout *C.struct_timespec) (C.int, error) {
	rc, err := C.semop(semid, sops, nsops)
	return rc, err
}
