//go:build !darwin

package netutil

type IPv6Status struct {
	ServiceName string
	Device      string
	IsAutomatic bool
	CurrentMode string
}

func CheckMacOSIPv6Status() (*IPv6Status, error) {
	return nil, nil
}

func EnsureMacOSIPv6Automatic() (*IPv6Status, bool, error) {
	return nil, false, nil
}
