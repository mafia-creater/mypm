//go:build windows

package linker

import (
	"errors"
	"os"
	"syscall"
)

// isCrossDevice returns true when a hard link fails because of a
// cross-volume error on Windows (ERROR_NOT_SAME_DEVICE = 17).
func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		if errno, ok := linkErr.Err.(syscall.Errno); ok {
			// ERROR_NOT_SAME_DEVICE
			return errno == 17
		}
	}
	return false
}
