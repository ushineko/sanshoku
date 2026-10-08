package sanshoku_test

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku"
)

// Windows refuses an open with ERROR_ACCESS_DENIED, which is neither EACCES
// nor EPERM; a device another program holds unshared must still read as not
// permitted rather than absent (#39).
func TestAnAccessDeniedOpenOnWindowsIsAPermissionError(t *testing.T) {
	err := &os.PathError{Op: "open", Path: `\?\hid#example`, Err: syscall.ERROR_ACCESS_DENIED}

	assert.True(t, sanshoku.IsPermission(err))
	assert.False(t, sanshoku.IsPermission(&os.PathError{Op: "open", Path: `\?\hid#example`, Err: syscall.ERROR_FILE_NOT_FOUND}))
}
