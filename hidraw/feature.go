package hidraw

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

/*
The hidraw feature-report ioctls, composed here because x/sys/unix does not
name them.

They are `_IOC(READ|WRITE, 'H', 0x06|0x07, len)`: the length is part of the
request number, so the request is a function of the buffer and not a constant
at all. Getting the direction bits the wrong way round is the classic mistake
and shows up as EINVAL rather than as anything informative.
*/
const (
	iocWrite uint64 = 1
	iocRead  uint64 = 2

	iocTypeShift uint64 = 8
	iocSizeShift uint64 = 16
	iocDirShift  uint64 = 30

	// iocSizeMax is the widest report a request number can carry: the size
	// field is fourteen bits. A longer buffer would silently wrap into the
	// direction bits and address some other ioctl entirely, so it is refused
	// instead.
	iocSizeMax uint64 = 1<<14 - 1

	hidrawMagic uint64 = 'H'

	nrSetFeature uint64 = 0x06
	nrGetFeature uint64 = 0x07
)

func ioc(dir, typ, nr, size uint64) uint64 {
	return dir<<iocDirShift | size<<iocSizeShift | typ<<iocTypeShift | nr
}

/*
SetFeature sends a feature report, report ID first. It is how a device that
declares no output report is spoken to: a Razer dock has nowhere for a hidraw
write to go, and its exchange is a pair of feature ioctls instead.

**The ioctl cannot be interrupted.** It runs in the calling goroutine, with the
context checked before and after; a context that expires meanwhile is reported
as its error once the ioctl returns. A device that hangs the ioctl hangs the
caller. hayami has read a dock this way for months without one.
*/
func (h *Handle) SetFeature(ctx context.Context, report []byte) error {
	return h.feature(ctx, nrSetFeature, report)
}

// GetFeature reads a feature report into report, whose first byte is the
// report ID to ask for. The bounds on the call are those of SetFeature.
func (h *Handle) GetFeature(ctx context.Context, report []byte) error {
	return h.feature(ctx, nrGetFeature, report)
}

func (h *Handle) feature(ctx context.Context, nr uint64, b []byte) error {
	if len(b) == 0 {
		return errors.New("an empty hidraw feature report")
	}
	size := uint64(len(b))
	if size > iocSizeMax {
		return fmt.Errorf("a hidraw feature report of %d bytes", len(b))
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	/*
		Through RawControl and not Fd. Fd puts the file into blocking mode,
		after which SetReadDeadline stops working, and this handle's reads
		depend on it.
	*/
	conn, err := h.r.SyscallConn()
	if err != nil {
		return fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	request := uintptr(ioc(iocRead|iocWrite, hidrawMagic, nr, size))
	var errno unix.Errno
	if err := conn.Control(func(fd uintptr) {
		// The buffer's address is the ioctl's third argument, which is what
		// a hidraw feature report is; b stays live across the call.
		_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(&b[0]))) //nolint:gosec // the documented hidraw ioctl
	}); err != nil {
		return fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	if errno != 0 {
		return h.fault("feature report on", errno)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	return nil
}
