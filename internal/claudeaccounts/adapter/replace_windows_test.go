//go:build windows

package claudecli

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestTemporaryWindowsLock(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.ERROR_ACCESS_DENIED, 32, 33} {
		err := &os.LinkError{Op: "rename", Old: "from", New: "to", Err: errno}
		if !temporaryWindowsLock(err) {
			t.Errorf("erro %d devia permitir nova tentativa", errno)
		}
	}
	if temporaryWindowsLock(errors.New("disco cheio")) {
		t.Error("erro permanente não pode ficar repetindo")
	}
}
