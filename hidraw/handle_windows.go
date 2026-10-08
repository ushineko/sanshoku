package hidraw

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ushineko/sanshoku"
)

/*
Handle is an open HID interface: every top-level collection of it, opened
together.

Requests are routed and replies gathered so that a driver sees one node, as it
does on Linux (spec 012):

  - **A write goes to the collection that declares its report ID** as an
    output report. A Logitech short request (0x10) goes to the short
    collection and a long one (0x11) to the long.
  - **A read takes the first report from any collection.** Each collection
    that can be read keeps one overlapped read waiting, and Read returns
    whichever finishes first. A HID++ short request answered with a long
    report is answered on a different collection from the one asked, and
    reading only the one written to would wait out the timeout instead.
  - **A feature report goes to the collection that declares it**, or failing
    that to the first whose feature reports are long enough.

Each collection is opened for reading and writing where Windows allows it,
and with no access at all where it does not: Windows owns a keyboard's or a
mouse's collection and will not share it, but a feature report still reaches
one opened with no access (measured on a Razer mouse's interface 0, spec 012),
and that is what a Razer battery is read through.
*/
type Handle struct {
	name  string
	mu    sync.Mutex
	colls []*collection
	// queue is reports that arrived while another was being taken. Each
	// collection's read finishes on its own, and one that finished is
	// not lost because Read returned another's.
	queue [][]byte
	// present says whether the interface is still there, for telling an
	// unplugged device from a failing one.
	present func() bool
}

// collection is one top-level collection of a Handle: what it declares, and
// the channel to it.
type collection struct {
	info collectionInfo
	ch   channel
}

/*
channel is the I/O a Handle does on one collection: an overlapped file in
use, a test's stand-in under test.

arm starts a read if none is waiting; ready is the event that is signalled
when the waiting read has finished; take is that read's report, after which
the channel is idle. A channel that cannot be read (opened with no access)
says so with readable, and is never armed.
*/
type channel interface {
	readable() bool
	arm() error
	ready() windows.Handle
	take() ([]byte, error)
	write(report []byte, wait time.Duration) error
	ioctl(code uint32, in, out []byte, wait time.Duration) (int, error)
	close() error
}

// Open opens every collection of the interface path belongs to: the node's
// Path, or any one of its collections' paths.
func Open(path string) (*Handle, error) {
	first, err := describe(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	members := []collectionInfo{first}
	if first.parent != "" {
		paths, err := interfaces()
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		for _, p := range paths {
			if p == path {
				continue
			}
			if info, err := describe(p); err == nil && info.parent == first.parent {
				members = append(members, info)
			}
		}
	}

	h := &Handle{name: path, present: func() bool { return listed(path) }}
	for _, m := range members {
		f, err := openFile(m)
		if err != nil {
			continue
		}
		h.colls = append(h.colls, &collection{info: m, ch: f})
	}
	if len(h.colls) == 0 {
		return nil, fmt.Errorf("opening %s: no collection of it would open", path)
	}
	return h, nil
}

// listed reports whether a collection is still among the present ones.
func listed(path string) bool {
	paths, err := interfaces()
	if err != nil {
		return true // cannot tell, so do not claim it gone
	}
	return slices.Contains(paths, path)
}

// Close closes every collection.
func (h *Handle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for _, c := range h.colls {
		errs = append(errs, c.ch.close())
	}
	h.colls = nil
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("closing %s: %w", h.name, err)
	}
	return nil
}

// Write sends one output report, report ID first where the device numbers
// its reports and zero first where it does not, as on Linux. It is padded to
// the collection's output length, which Windows requires a write to be.
func (h *Handle) Write(report []byte) error {
	if len(report) == 0 {
		return errors.New("an empty output report")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	c, framed, err := route(h.colls, ReportOutput, report)
	if err != nil {
		return fmt.Errorf("writing to %s: %w", h.name, err)
	}
	if err := c.ch.write(framed, defaultWait); err != nil {
		return h.fault("writing to", err)
	}
	return nil
}

/*
Read reads one input report into buf, from whichever collection has one
first.

The deadline is the context's; a context with no deadline is given two
seconds. A report that does not arrive in time is context.DeadlineExceeded,
and an unplugged interface is sanshoku.ErrGone, as on Linux. A collection
that does not number its reports has the zero Windows puts in front of every
report taken off, so that what a driver reads is what hidraw would have
handed it.
*/
func (h *Handle) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("reading %s: %w", h.name, err)
	}
	when, ok := ctx.Deadline()
	if !ok {
		when = time.Now().Add(defaultWait)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	report, err := h.next(when)
	if err != nil {
		return 0, err
	}
	return copy(buf, report), nil
}

// next is the next report, queued or read, waiting no later than when.
func (h *Handle) next(when time.Time) ([]byte, error) {
	if len(h.queue) > 0 {
		r := h.queue[0]
		h.queue = h.queue[1:]
		return r, nil
	}
	var events []windows.Handle
	var waiting []*collection
	for _, c := range h.colls {
		if !c.ch.readable() {
			continue
		}
		if err := c.ch.arm(); err != nil {
			return nil, h.fault("reading", err)
		}
		events = append(events, c.ch.ready())
		waiting = append(waiting, c)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("reading %s: no collection of it can be read", h.name)
	}
	wait := time.Until(when)
	if wait < 0 {
		wait = 0
	}
	got, err := windows.WaitForMultipleObjects(events, false, uint32(wait.Milliseconds())) //nolint:gosec // bounded by defaultWait or the caller's deadline
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", h.name, err)
	}
	if got == uint32(windows.WAIT_TIMEOUT) {
		return nil, fmt.Errorf("no report from %s: %w", h.name, context.DeadlineExceeded)
	}
	i := int(got - windows.WAIT_OBJECT_0)
	if i < 0 || i >= len(waiting) {
		return nil, fmt.Errorf("reading %s: wait returned %#x", h.name, got)
	}
	c := waiting[i]
	report, err := c.ch.take()
	if err != nil {
		return nil, h.fault("reading", err)
	}
	return unframe(c.info, report), nil
}

// Drain discards up to depth reports that arrived before the question was
// asked, for the reason the Linux Drain gives. Windows queues reports per
// collection and per handle, as Linux does per handle.
func (h *Handle) Drain(depth int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queue = nil
	for range depth {
		if _, err := h.next(time.Now().Add(drainWait)); err != nil {
			return
		}
	}
}

/*
SetFeature sends a feature report, report ID first, to the collection that
declares it, padded to that collection's feature length as Windows requires.

Unlike the Linux ioctl it has a deadline: the request is overlapped, and one
the device has not finished with by the context's deadline (two seconds where
the context has none) is cancelled and reported as context.DeadlineExceeded.
*/
func (h *Handle) SetFeature(ctx context.Context, report []byte) error {
	_, err := h.feature(ctx, ioctlSetFeature, report)
	return err
}

// GetFeature reads a feature report into report, whose first byte is the
// report ID to ask for. The bounds on the call are those of SetFeature.
func (h *Handle) GetFeature(ctx context.Context, report []byte) error {
	got, err := h.feature(ctx, ioctlGetFeature, report)
	if err != nil {
		return err
	}
	copy(report, got)
	return nil
}

func (h *Handle) feature(ctx context.Context, code uint32, report []byte) ([]byte, error) {
	if len(report) == 0 {
		return nil, errors.New("an empty feature report")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	wait := defaultWait
	if when, ok := ctx.Deadline(); ok {
		wait = max(time.Until(when), 0)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	c, framed, err := route(h.colls, ReportFeature, report)
	if err != nil {
		return nil, fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	var out []byte
	if code == ioctlGetFeature {
		out = framed
	}
	n, err := c.ch.ioctl(code, framed, out, wait)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("feature report on %s: %w", h.name, err)
		}
		return nil, h.fault("feature report on", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("feature report on %s: %w", h.name, err)
	}
	return framed[:min(n, len(framed))], nil
}

/*
route picks the collection a report of kind goes to and frames the report for
it: report ID first, padded with zeros to the collection's length for the
kind.

A collection that numbers its reports takes the reports it declares. One that
does not takes any report, framed as hidraw frames one: a leading zero is the
"no report ID" byte and stays; a report that starts with anything else is all
data, as hidraw writes it, and Windows is given the zero in front that it asks
for. A feature report numbered nowhere goes to the first collection whose
feature reports are long enough to hold it.
*/
func route(colls []*collection, kind ReportKind, report []byte) (*collection, []byte, error) {
	id := report[0]
	var target *collection
	unnumbered := false
	for _, c := range colls {
		if c.info.declares(kind, id) && id != 0 {
			target = c
			break
		}
	}
	if target == nil {
		for _, c := range colls {
			if c.info.length(kind) > 0 && !c.info.numbered(kind) {
				target, unnumbered = c, true
				break
			}
		}
	}
	if target == nil && kind == ReportFeature {
		for _, c := range colls {
			if c.info.length(kind) >= len(report) {
				target = c
				break
			}
		}
	}
	if target == nil {
		return nil, nil, fmt.Errorf("no collection takes %s report %#02x", kind, id)
	}
	framed := report
	if unnumbered && id != 0 {
		framed = append([]byte{0}, report...)
	}
	size := target.info.length(kind)
	if len(framed) > size {
		return nil, nil, fmt.Errorf("a %s report of %d bytes, where the collection takes %d", kind, len(framed), size)
	}
	padded := make([]byte, size)
	copy(padded, framed)
	return target, padded, nil
}

// unframe takes off the zero Windows puts in front of a report from a
// collection that does not number its input reports, which hidraw does not
// hand over.
func unframe(info collectionInfo, report []byte) []byte {
	if len(report) > 0 && !info.numbered(ReportInput) {
		return report[1:]
	}
	return report
}

// declares reports whether the collection declares a report of kind numbered
// id.
func (c collectionInfo) declares(kind ReportKind, id byte) bool {
	for _, r := range c.reports {
		if r.Kind == kind && r.ID == id {
			return true
		}
	}
	return false
}

// numbered reports whether the collection numbers its reports of kind.
func (c collectionInfo) numbered(kind ReportKind) bool {
	for _, r := range c.reports {
		if r.Kind == kind && r.ID != 0 {
			return true
		}
	}
	return false
}

// length is the collection's report length for kind, in bytes with the ID.
func (c collectionInfo) length(kind ReportKind) int {
	switch kind {
	case ReportInput:
		return c.inLen
	case ReportOutput:
		return c.outLen
	case ReportFeature:
		return c.featLen
	default:
		return 0
	}
}

/*
fault wraps a read, write or feature error, and reports an unplugged interface
as sanshoku.ErrGone.

ERROR_DEVICE_NOT_CONNECTED, ERROR_NO_SUCH_DEVICE and ERROR_DEVICE_REMOVED are
Windows saying so directly. ERROR_GEN_FAILURE and ERROR_OPERATION_ABORTED are
what a request in flight gets when the device leaves, and also what a device
that is there and refusing gets, so they count as gone only when the
collection is no longer listed -- the same rule the Linux fault applies to
EIO.
*/
func (h *Handle) fault(doing string, err error) error {
	switch {
	case errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED), errors.Is(err, windows.ERROR_NO_SUCH_DEVICE),
		errors.Is(err, windows.ERROR_DEVICE_REMOVED):
		return fmt.Errorf("%s %s: %w: %w", doing, h.name, sanshoku.ErrGone, err)
	case (errors.Is(err, windows.ERROR_GEN_FAILURE) || errors.Is(err, windows.ERROR_OPERATION_ABORTED)) &&
		h.present != nil && !h.present():
		return fmt.Errorf("%s %s: %w: %w", doing, h.name, sanshoku.ErrGone, err)
	}
	return fmt.Errorf("%s %s: %w", doing, h.name, err)
}
