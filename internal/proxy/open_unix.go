//go:build !windows

package proxy

import (
	"os"
	"syscall"
)

func openRegularNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
