package usbfs

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

/*
Enough of Linux usbfs to claim one interface and write to a bulk endpoint.

No libusb and no cgo: these are ioctls on /dev/bus/usb/BBB/DDD, and the whole
of what the Kraken's LCD needs is three of them. The screen's data does not go
over HID (that interface carries control only), so this is the other half of
talking to the cooler.
*/
const (
	claimInterface   = 0x8004550F // _IOR('U', 15, unsigned int)
	releaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	bulkTransfer     = 0xC0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
)

/*
chunk is how much of a write goes in one bulk transfer.

Not a limit on what can be sent (the Kraken's panel holds about 24 MB and an
animation happily fills it) but on what one ioctl is asked to carry. usbfs
allocates a single contiguous buffer per transfer, so a 19 MB write is a 19 MB
kernel allocation; splitting it costs nothing on a stream endpoint.

A multiple of the 1024-byte packet the Kraken's protocol counts in, so a chunk
boundary is never inside a packet.

Found by hotaru: four of its nine animations are between 9 and 20 MB, and every
one of them failed on a cap that only ever saw a 22 KB dashboard frame.
*/
const chunk = 1 << 20

// defaultTimeout bounds a transfer whose context has no deadline.
const defaultTimeout = 5 * time.Second

// bulk is struct usbdevfs_bulktransfer, laid out as the kernel expects.
type bulk struct {
	endpoint uint32
	length   uint32
	timeout  uint32 // milliseconds
	_        uint32 // padding, so the pointer lands on an eight-byte boundary
	data     uintptr
}

// Interface is a claimed interface on a USB device.
type Interface struct {
	f     *os.File
	iface uint32
	name  string
}

/*
Open claims an interface on a USB device node.

Claiming is what stops two programs writing to the endpoint at once. No kernel
driver is detached: the Kraken's LCD interface has none bound, so usbhid keeps
the HID interface beside it and both are open at once. A device that needs a
detach gets it in its own spec.
*/
func Open(path string, iface int) (*Interface, error) {
	if iface < 0 || iface > math.MaxUint8 {
		return nil, fmt.Errorf("interface %d is not a USB interface number", iface)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	u := &Interface{f: f, iface: uint32(iface), name: path}
	n := u.iface
	if err := u.ioctl(claimInterface, unsafe.Pointer(&n)); err != nil { //nolint:gosec // the usbdevfs calling convention; n outlives the call
		_ = f.Close()
		return nil, fmt.Errorf("claiming interface %d of %s: %w", iface, path, err)
	}
	return u, nil
}

/*
Bulk writes data to an OUT endpoint, in chunks.

The context's deadline becomes each transfer's timeout, recomputed per chunk;
a context with no deadline gives each transfer five seconds. The count the
kernel reports is checked rather than assumed: a short write means the device
received part of an image and will draw whatever was already in that memory
for the rest, which looks like a corrupted panel and reports as a success.
*/
func (u *Interface) Bulk(ctx context.Context, endpoint byte, data []byte) error {
	for {
		n := min(len(data), chunk)
		if err := u.transfer(ctx, endpoint, data[:n]); err != nil {
			return err
		}
		data = data[n:]
		if len(data) == 0 {
			return nil
		}
	}
}

// transfer is one bulk write, which the kernel does in one allocation,
// bounded by chunk.
func (u *Interface) transfer(ctx context.Context, endpoint byte, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	ms, err := timeout(ctx)
	if err != nil {
		return fmt.Errorf("writing to endpoint %#02x: %w", endpoint, err)
	}
	t := bulk{
		endpoint: uint32(endpoint),
		length:   uint32(len(data)), //nolint:gosec // bounded by chunk
		timeout:  ms,
		data:     uintptr(unsafe.Pointer(&data[0])), //nolint:gosec // the usbdevfs calling convention; data outlives the call
	}
	sent, errno, err := u.syscall(bulkTransfer, unsafe.Pointer(&t)) //nolint:gosec // as above
	if err != nil {
		return fmt.Errorf("writing to endpoint %#02x of %s: %w", endpoint, u.name, err)
	}
	if errno != 0 {
		return fmt.Errorf("writing %d bytes to endpoint %#02x of %s: %w", len(data), endpoint, u.name, errno)
	}
	if int(sent) != len(data) { //nolint:gosec // sent is at most len(data)
		return fmt.Errorf("short write to endpoint %#02x: sent %d of %d", endpoint, sent, len(data))
	}
	return nil
}

/*
timeout is the context's remaining time in the milliseconds the ioctl takes.

At least one: the kernel reads zero as "wait forever", which is the one value
this must never pass. At most what the field holds.
*/
func timeout(ctx context.Context) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err //nolint:wrapcheck // the caller wraps it
	}
	left := defaultTimeout
	if when, ok := ctx.Deadline(); ok {
		left = time.Until(when)
	}
	ms := left.Milliseconds()
	switch {
	case ms < 1:
		return 1, nil
	case ms > math.MaxUint32:
		return math.MaxUint32, nil
	}
	return uint32(ms), nil
}

// Close releases the interface and closes the device node.
func (u *Interface) Close() error {
	n := u.iface
	_ = u.ioctl(releaseInterface, unsafe.Pointer(&n)) //nolint:gosec // as above
	if err := u.f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", u.name, err)
	}
	return nil
}

// ioctl passes a pointer to a local value that outlives the call.
func (u *Interface) ioctl(request uintptr, arg unsafe.Pointer) error {
	_, errno, err := u.syscall(request, arg)
	if err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}

/*
syscall makes one ioctl on the device node.

Through RawControl and not Fd, as in hidraw: Fd puts the file into blocking
mode, after which deadlines stop working. Nothing here reads with a deadline,
so it would be harmless today; it is done the same way so that nobody has to
work out whether it matters.
*/
func (u *Interface) syscall(request uintptr, arg unsafe.Pointer) (uintptr, unix.Errno, error) {
	conn, err := u.f.SyscallConn()
	if err != nil {
		return 0, 0, fmt.Errorf("ioctl on %s: %w", u.name, err)
	}
	var (
		r1    uintptr
		errno unix.Errno
	)
	if err := conn.Control(func(fd uintptr) {
		r1, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, request, uintptr(arg))
	}); err != nil {
		return 0, 0, fmt.Errorf("ioctl on %s: %w", u.name, err)
	}
	return r1, errno, nil
}
