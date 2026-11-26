//go:build !darwin && !linux && !windows

package service

// NewManager returns ErrUnsupportedPlatform on operating systems without a native service backend.
func NewManager() (Manager, error) {
	return nil, ErrUnsupportedPlatform
}
