//go:build !darwin

package cockpit

import "time"

// IsProxyPortListening is a stub on non-Darwin platforms.
func IsProxyPortListening(port int, timeout time.Duration) bool {
	return false
}

// GetPrimaryNetworkService is a stub on non-Darwin platforms.
func GetPrimaryNetworkService() (string, error) {
	return "", nil
}

// EnsureSystemProxy is a no-op on non-Darwin platforms.
func EnsureSystemProxy(targetPort int) (bool, error) {
	return false, nil
}
