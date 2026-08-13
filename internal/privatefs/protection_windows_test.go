//go:build windows

package privatefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, DirMode); err != nil {
		t.Fatal(err)
	}
	if err := Restrict(path, DirMode); err != nil {
		t.Fatal(err)
	}
	private, observed, err := Inspect(path, DirMode)
	if err != nil {
		t.Fatal(err)
	}
	if !private {
		t.Fatalf("directory protection is not private (mode %o): %s", observed, describeWindowsACL(path))
	}
}

func describeWindowsACL(path string) string {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return "read descriptor: " + err.Error()
	}
	if descriptor == nil {
		return "descriptor is nil"
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return "read DACL: " + fmt.Sprint(err)
	}
	entries := make([]string, 0, dacl.AceCount)
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			entries = append(entries, fmt.Sprintf("ace[%d]=%v", index, err))
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		entries = append(entries, fmt.Sprintf("ace[%d]=type:%d flags:%x mask:%x sid:%s", index, ace.Header.AceType, ace.Header.AceFlags, ace.Mask, sid.String()))
	}
	return descriptor.String() + "; " + strings.Join(entries, "; ")
}
