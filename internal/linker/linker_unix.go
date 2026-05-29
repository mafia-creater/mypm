//go:build !windows

package linker

import (
	"errors"
	"os"
	"syscall"
)

// isCrossDevice returns true when a hard link fails because src and dest
// are on different filesystems (EXDEV on Linux/macOS).
func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		if errno, ok := linkErr.Err.(syscall.Errno); ok {
			return errno == syscall.EXDEV
		}
	}
	return false
}
