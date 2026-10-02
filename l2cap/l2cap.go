//go:build linux

package l2cap

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

/*
defaultWait bounds a dial or a receive whose caller set no deadline.

It is hayami's AAPTimeout, the bound it put on the whole accessory-protocol
exchange: a device that connects and never reports must not take the caller's
poll loop with it. The real exchange finished well inside a second on the
AirPods Pro hayami measured.
*/
const defaultWait = 6 * time.Second

// maxPollWait caps one wait, so the conversion into poll's argument cannot
// overflow however long a caller asks for.
const maxPollWait = int32(60_000)

// packetSize is the buffer one packet is read into. Every accessory-protocol
// packet hayami saw fitted in far less; a sequenced packet longer than this is
// truncated by the kernel, not split.
const packetSize = 1024

/*
Conn is an open L2CAP sequenced-packet channel.

Each Send is one packet and each Receive returns one. A Conn is safe for
concurrent use; the caller keeps its conversations apart.
*/
type Conn struct {
	mu sync.Mutex
	fd int
}

/*
Dial opens an L2CAP channel to a device's port psm.

Two things here are not obvious and both of them read as the device refusing
the connection when they are wrong:

**The address goes in written order.** [unix.SockaddrL2] reverses it on the way
to the kernel, so the six bytes here are the six bytes of the printed address,
left to right. Reversing them first is what a raw sockaddr_l2 wants and what
every C example does, and doing it dials an address nothing answers on — for
which the kernel's answer is ECONNREFUSED. That is indistinguishable, from the
outside, from AirPods declining the channel.

**The connect completes asynchronously.** It reports EINPROGRESS and the real
answer arrives later, in SO_ERROR, once the socket is writable. Treating
EINPROGRESS as the failure reports a working device as broken.

The socket is non-blocking for the connect, so the context's deadline bounds it
(six seconds when the context has none), and blocking again once connected.
The error names the port and never the address.
*/
func Dial(ctx context.Context, addr [6]byte, psm uint16) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("dialling l2cap port %#04x: %w", psm, err)
	}
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_SEQPACKET|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.BTPROTO_L2CAP)
	if err != nil {
		return nil, fmt.Errorf("opening an l2cap socket: %w", err)
	}

	err = unix.Connect(fd, &unix.SockaddrL2{PSM: psm, Addr: addr})
	// A non-blocking connect interrupted by a signal carries on in the
	// background exactly as one that reported EINPROGRESS does.
	if errors.Is(err, unix.EINPROGRESS) || errors.Is(err, unix.EINTR) {
		err = awaitConnect(ctx, fd)
	}
	if err == nil {
		err = unix.SetNonblock(fd, false)
	}
	if err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("connecting to l2cap port %#04x: %w", psm, err)
	}
	return &Conn{fd: fd}, nil
}

// awaitConnect waits for an asynchronous connect and reports how it went.
func awaitConnect(ctx context.Context, fd int) error {
	n, err := poll(fd, unix.POLLOUT, deadline(ctx))
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("the connect did not complete in time: %w", context.DeadlineExceeded)
	}

	code, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
	if err != nil {
		return fmt.Errorf("asking how the connect went: %w", err)
	}
	if code != 0 {
		return unix.Errno(code)
	}
	return nil
}

// Send writes one packet.
func (c *Conn) Send(p []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return fmt.Errorf("sending on an l2cap channel: %w", unix.EBADF)
	}
	for {
		_, err := unix.Write(c.fd, p)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("sending on an l2cap channel: %w", err)
		}
		return nil
	}
}

/*
Receive waits for one packet until the context's deadline, six seconds when it
has none.

Nothing arriving in time is an error wrapping context.DeadlineExceeded, which a
caller waiting through a device's chatter treats as silence rather than as a
fault. Cancelling the context does not interrupt a wait already begun; the
deadline bounds it.
*/
func (c *Conn) Receive(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("waiting on an l2cap channel: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return nil, fmt.Errorf("waiting on an l2cap channel: %w", unix.EBADF)
	}

	n, err := poll(c.fd, unix.POLLIN, deadline(ctx))
	if err != nil {
		return nil, fmt.Errorf("waiting on an l2cap channel: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("no packet on the l2cap channel: %w", context.DeadlineExceeded)
	}

	buf := make([]byte, packetSize)
	var read int
	for {
		read, err = unix.Read(c.fd, buf)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading an l2cap channel: %w", err)
	}
	return buf[:read], nil
}

// Close ends the conversation. Closing twice is not an error.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fd < 0 {
		return nil
	}
	err := unix.Close(c.fd)
	c.fd = -1
	if err != nil {
		return fmt.Errorf("closing an l2cap channel: %w", err)
	}
	return nil
}

// deadline is the context's deadline, or defaultWait from now.
func deadline(ctx context.Context) time.Time {
	if when, ok := ctx.Deadline(); ok {
		return when
	}
	return time.Now().Add(defaultWait)
}

/*
poll waits for a socket, retrying the interruptions Go causes itself.

**EINTR is not a failure here and it is not rare.** The Go runtime preempts
goroutines with signals, and a signal delivered while poll(2) is waiting
returns EINTR. Treating that as an error made the accessory protocol fail
against a device that was answering perfectly well, with a message — "waiting
on the accessory channel: interrupted system call" — that says nothing about
what actually happened.

The deadline is recomputed on each retry so the interruptions cannot extend
the wait past what the caller asked for. Zero ready descriptors is the
deadline passing.
*/
func poll(fd int, events int16, until time.Time) (int, error) {
	if fd < 0 || fd > math.MaxInt32 {
		return 0, fmt.Errorf("%d is not a file descriptor", fd)
	}
	for {
		left := time.Until(until)
		if left <= 0 {
			return 0, nil
		}

		// The wait is in milliseconds, rounded up so a sub-millisecond
		// remainder waits rather than spinning, and capped so it cannot reach
		// the width of the argument.
		ms := (left + time.Millisecond - 1).Milliseconds()
		if ms > int64(maxPollWait) {
			ms = int64(maxPollWait)
		}

		pfd := []unix.PollFd{{Fd: int32(fd), Events: events}} //nolint:gosec // bounded above
		n, err := unix.Poll(pfd, int(ms))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("waiting on an l2cap socket: %w", err)
		}
		if n == 0 && time.Until(until) > 0 {
			// The cap, not the deadline, ended this wait.
			continue
		}
		return n, nil
	}
}
