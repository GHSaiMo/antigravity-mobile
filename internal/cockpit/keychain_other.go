//go:build !windows

package cockpit

import "fmt"

func writeWindowsGenericCredential(_, _, _ string) error {
	return fmt.Errorf("windows credential manager unsupported on this platform")
}
