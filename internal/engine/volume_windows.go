//go:build windows

package engine

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetVolumePathNameW    = kernel32.NewProc("GetVolumePathNameW")
	errGetVolumePathNameEmpty = fmt.Errorf("GetVolumePathName returned an empty path")
)

// volumeOf asks Windows which volume holds the path. The answer is a mount
// root such as `C:\`, which is the unit a shadow copy covers.
func volumeOf(path string) (string, error) {
	wide, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("volume of %q: %w", path, err)
	}
	buf := make([]uint16, syscall.MAX_PATH+1)
	ret, _, callErr := procGetVolumePathNameW.Call(
		uintptr(unsafe.Pointer(wide)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return "", fmt.Errorf("volume of %q: %w", path, callErr)
	}
	out := syscall.UTF16ToString(buf)
	if out == "" {
		return "", fmt.Errorf("volume of %q: %w", path, errGetVolumePathNameEmpty)
	}
	return out, nil
}
