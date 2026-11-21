//go:build windows

package cockpit

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

type winCredentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	modAdvapi32    = syscall.NewLazyDLL("advapi32.dll")
	procCredWriteW = modAdvapi32.NewProc("CredWriteW")
)

func writeWindowsGenericCredential(target, user, secret string) error {
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	userPtr, err := syscall.UTF16PtrFromString(user)
	if err != nil {
		return err
	}
	blob := []byte(secret)
	var blobPtr *byte
	if len(blob) > 0 {
		blobPtr = &blob[0]
	}
	cred := winCredentialW{
		Type:               credTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     blobPtr,
		Persist:            credPersistLocalMachine,
		UserName:           userPtr,
	}
	r1, _, e1 := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	runtime.KeepAlive(targetPtr)
	runtime.KeepAlive(userPtr)
	runtime.KeepAlive(blob)
	if r1 == 0 {
		if e1 != nil && e1 != syscall.Errno(0) {
			return fmt.Errorf("CredWriteW failed: %w", e1)
		}
		return fmt.Errorf("CredWriteW returned 0")
	}
	return nil
}
