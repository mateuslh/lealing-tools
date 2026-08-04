//go:build windows

package claudecli

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// replaceFile tolera a janela curta em que o Claude Code, o indexador ou o
// antivírus ainda mantém o arquivo aberto. O MoveFileEx usado por os.Rename
// não consegue substituir o destino durante essa janela no Windows.
func replaceFile(from, to string) error {
	deadline := time.Now().Add(2 * time.Second)
	delay := 10 * time.Millisecond
	for {
		err := os.Rename(from, to)
		if err == nil || !temporaryWindowsLock(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, 200*time.Millisecond)
	}
}

func temporaryWindowsLock(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) ||
		errors.Is(err, syscall.Errno(32)) || // ERROR_SHARING_VIOLATION
		errors.Is(err, syscall.Errno(33)) // ERROR_LOCK_VIOLATION
}
