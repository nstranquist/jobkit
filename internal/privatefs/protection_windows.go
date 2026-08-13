//go:build windows

package privatefs

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func restrictPath(path string, mode os.FileMode) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	inheritance := ""
	if mode == DirMode {
		inheritance = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		fmt.Sprintf("D:P(A;%s;GA;;;%s)", inheritance, user.String()),
	)
	if err != nil {
		return fmt.Errorf("build private access-control list: %w", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read private access-control list: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("set private access-control list: %w", err)
	}
	return nil
}

func inspectPath(path string, want os.FileMode) (bool, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	observed := info.Mode().Perm()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return false, observed, err
	}
	if descriptor == nil {
		return false, observed, nil
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false, observed, err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return false, observed, err
	}
	user, err := currentUserSID()
	if err != nil {
		return false, observed, err
	}
	allowed := false
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return false, observed, err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return false, observed, nil
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user) {
			return false, observed, nil
		}
		if want == DirMode {
			inherit := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
			if ace.Header.AceFlags&inherit != inherit {
				return false, observed, nil
			}
		}
		allowed = true
	}
	return allowed, observed, nil
}

func currentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current Windows user: %w", err)
	}
	return user.User.Sid, nil
}
