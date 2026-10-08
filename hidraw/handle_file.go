//go:build !windows

package hidraw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ushineko/sanshoku"
)

// drainLen is the buffer a drain reads into. hidraw hands over one whole
// report per read and discards what does not fit, so its size does not
// decide how much is drained.
const drainLen = 64

/*
Handle is an open hidraw node.

A node is one file opened read-write. The read side and the write side are
held as two fields so that a test can put a pipe pair where the node would be;
for a node they are the same file.
*/
type Handle struct {
	r, w *os.File
	name string
}

// Open opens a hidraw node for reading and writing.
func Open(path string) (*Handle, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Handle{r: f, w: f, name: path}, nil
}

// Close closes the node.
func (h *Handle) Close() error {
	err := h.r.Close()
	if h.w != h.r {
		err = errors.Join(err, h.w.Close())
	}
	if err != nil {
		return fmt.Errorf("closing %s: %w", h.name, err)
	}
	return nil
}

// Write sends one output report, report ID first where the device numbers
// its reports.
func (h *Handle) Write(report []byte) error {
	if _, err := h.w.Write(report); err != nil {
		return h.fault("writing to", err)
	}
	return nil
}

/*
Read reads one input report into buf.

The deadline is the context's, pushed down to the file; checking the context
between reads is not enough on its own, because a read on a node that never
answers blocks until something ends it. A context with no deadline is given
two seconds. A report that does not arrive in time is context.DeadlineExceeded.
A node that has been unplugged is sanshoku.ErrGone.
*/
func (h *Handle) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("reading %s: %w", h.name, err)
	}
	when, ok := ctx.Deadline()
	if !ok {
		when = time.Now().Add(defaultWait)
	}
	if err := h.r.SetReadDeadline(when); err != nil {
		return 0, fmt.Errorf("bounding the wait on %s: %w", h.name, err)
	}
	n, err := h.r.Read(buf)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return n, fmt.Errorf("no report from %s: %w", h.name, context.DeadlineExceeded)
		}
		return n, h.fault("reading", err)
	}
	return n, nil
}

/*
Drain discards up to depth reports that arrived before the question was asked.

A device that broadcasts its state unasked (the Kraken does about once a
second) fills the kernel's queue for every open handle, so the queue is as deep
as the handle has been idle: a minute of quiet leaves sixty stale reports ahead
of the next reply. liquidctl calls the same thing clear_enqueued_reports and
does it before every status read. Pass QueueDepth to empty it.

Errors are ignored on purpose: a queue that cannot be drained is a device that
will fail the read that follows, and reporting it twice helps nobody.
*/
func (h *Handle) Drain(depth int) {
	if err := h.r.SetReadDeadline(time.Now().Add(drainWait)); err != nil {
		return
	}
	buf := make([]byte, drainLen)
	for range depth {
		if _, err := h.r.Read(buf); err != nil {
			return
		}
	}
}

/*
fault wraps a read or write error, and reports an unplugged node as
sanshoku.ErrGone.

ENODEV is the kernel saying so directly. EIO is what some paths return for a
device mid-removal as well as for a real I/O error, so it counts as gone only
when the node's sysfs entry has also gone.
*/
func (h *Handle) fault(doing string, err error) error {
	if errors.Is(err, syscall.ENODEV) || (errors.Is(err, syscall.EIO) && h.vanished()) {
		return fmt.Errorf("%s %s: %w: %w", doing, h.name, sanshoku.ErrGone, err)
	}
	return fmt.Errorf("%s %s: %w", doing, h.name, err)
}

// vanished reports whether the node's entry under SysRoot is missing.
func (h *Handle) vanished() bool {
	_, err := os.Stat(filepath.Join(SysRoot, filepath.Base(h.name)))
	return errors.Is(err, os.ErrNotExist)
}
