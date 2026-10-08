package hidraw

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

/*
file is a channel to one collection: a handle opened for overlapped I/O.

Its waiting read is the one piece of state that outlives a call. The buffer
and the OVERLAPPED belong to the kernel until the read finishes or is
cancelled, so both are pinned while it waits, and close cancels it and waits
for the cancellation to land before letting them go.
*/
type file struct {
	h       windows.Handle
	canRead bool
	event   windows.Handle
	ov      *windows.Overlapped
	buf     []byte
	pending bool
	pin     runtime.Pinner
}

// openFile opens a collection read-write, or with no access where Windows
// will not share it (a keyboard or a mouse it owns). A collection opened
// with no access cannot be read or written, and still takes feature reports.
func openFile(info collectionInfo) (*file, error) {
	f := &file{canRead: true}
	h, err := openCollection(info.path, windows.GENERIC_READ|windows.GENERIC_WRITE)
	if err != nil {
		f.canRead = false
		if h, err = openCollection(info.path, 0); err != nil {
			return nil, err
		}
	}
	f.h = h
	if info.inLen == 0 {
		f.canRead = false
	}
	if f.canRead {
		// Manual reset: the event stays signalled until arm starts the next
		// read, so a read that finished while another collection's was being
		// taken is still found on the next wait.
		ev, err := windows.CreateEvent(nil, 1, 0, nil)
		if err != nil {
			_ = windows.CloseHandle(h)
			return nil, fmt.Errorf("opening %s: %w", info.path, err)
		}
		f.event = ev
		f.buf = make([]byte, info.inLen)
	}
	return f, nil
}

func (f *file) readable() bool        { return f.canRead }
func (f *file) ready() windows.Handle { return f.event }

func (f *file) arm() error {
	if f.pending {
		return nil
	}
	if err := windows.ResetEvent(f.event); err != nil {
		return fmt.Errorf("resetting the read event: %w", err)
	}
	f.ov = &windows.Overlapped{HEvent: f.event}
	f.pin.Pin(f.ov)
	f.pin.Pin(&f.buf[0])
	err := windows.ReadFile(f.h, f.buf, nil, f.ov)
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		f.pin.Unpin()
		return fmt.Errorf("starting a read: %w", err)
	}
	// Finished at once or later, the event is set when it is done.
	f.pending = true
	return nil
}

func (f *file) take() ([]byte, error) {
	var n uint32
	err := windows.GetOverlappedResult(f.h, f.ov, &n, false)
	f.pending = false
	f.pin.Unpin()
	if err != nil {
		return nil, fmt.Errorf("finishing a read: %w", err)
	}
	out := make([]byte, n)
	copy(out, f.buf[:n])
	return out, nil
}

func (f *file) write(report []byte, wait time.Duration) error {
	return f.overlapped(wait, func(ov *windows.Overlapped, n *uint32) error {
		return windows.WriteFile(f.h, report, n, ov)
	}, nil)
}

func (f *file) ioctl(code uint32, in, out []byte, wait time.Duration) (int, error) {
	var got uint32
	err := f.overlapped(wait, func(ov *windows.Overlapped, n *uint32) error {
		var outPtr *byte
		if len(out) > 0 {
			outPtr = &out[0]
		}
		return windows.DeviceIoControl(f.h, code, &in[0], uint32(len(in)), outPtr, uint32(len(out)), n, ov) //nolint:gosec // report lengths are a few hundred bytes
	}, &got)
	return int(got), err
}

/*
overlapped runs one request and waits for it, no longer than wait.

A request the device has not finished by then is cancelled, and the
cancellation waited for, because the buffers stay the kernel's until it lands.
That is how a write or a feature report gets a deadline at all: the HidD_
calls have none.
*/
func (f *file) overlapped(wait time.Duration, start func(*windows.Overlapped, *uint32) error, done *uint32) error {
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return fmt.Errorf("creating an event: %w", err)
	}
	defer func() { _ = windows.CloseHandle(ev) }()
	ov := &windows.Overlapped{HEvent: ev}
	var n uint32
	var pin runtime.Pinner
	pin.Pin(ov)
	defer pin.Unpin()

	if err := start(ov, &n); err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return err
	}
	got, err := windows.WaitForSingleObject(ev, uint32(wait.Milliseconds())) //nolint:gosec // bounded by defaultWait or the caller's deadline
	if err != nil {
		return fmt.Errorf("waiting on the device: %w", err)
	}
	if got == uint32(windows.WAIT_TIMEOUT) {
		_ = windows.CancelIoEx(f.h, ov)
		_ = windows.GetOverlappedResult(f.h, ov, &n, true)
		return fmt.Errorf("the device did not finish in %v: %w", wait, context.DeadlineExceeded)
	}
	if err := windows.GetOverlappedResult(f.h, ov, &n, false); err != nil {
		return fmt.Errorf("finishing a request: %w", err)
	}
	if done != nil {
		*done = n
	}
	return nil
}

func (f *file) close() error {
	if f.pending {
		var n uint32
		_ = windows.CancelIoEx(f.h, f.ov)
		_ = windows.GetOverlappedResult(f.h, f.ov, &n, true)
		f.pending = false
		f.pin.Unpin()
	}
	var errs []error
	if f.event != 0 {
		errs = append(errs, windows.CloseHandle(f.event))
	}
	errs = append(errs, windows.CloseHandle(f.h))
	return errors.Join(errs...)
}
