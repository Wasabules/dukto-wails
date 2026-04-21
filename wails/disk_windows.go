//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// diskFree returns (available bytes, total bytes, nil) for the volume
// hosting path, via GetDiskFreeSpaceExW. "Available" is free bytes the
// current user can actually write (honouring per-user quotas).
func diskFree(path string) (uint64, uint64, error) {
	u16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")
	var freeToCaller, total, totalFree uint64
	r1, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(u16)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r1 == 0 {
		return 0, 0, callErr
	}
	return freeToCaller, total, nil
}
