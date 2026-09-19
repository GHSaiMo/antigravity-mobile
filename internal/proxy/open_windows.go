//go:build windows

package proxy

import (
	"fmt"
	"os"
)

func openRegularNoFollow(path string) (*os.File, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks not allowed: %s", path)
	}
	return os.Open(path)
}
