package hidraw

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/*
defaultWait bounds a read whose caller set no deadline.

Generous: a reply arrives in milliseconds on every device measured, and this is
the limit before a device that has stopped talking is reported as such rather
than waited on. The wrong hidraw node of a device never answers, and a read
that blocks forever is a program that hangs rather than one that reports
nothing there.
*/
const defaultWait = 2 * time.Second

/*
QueueDepth is how many reports the kernel will hold for one open handle.

Linux queues up to 64 per hidraw file description and drops the oldest beyond
that, so it bounds a Drain rather than guessing at one.
*/
const QueueDepth = 64

/*
drainWait is how long draining waits for a report that is not there.

Queued reports are readable immediately, so this is only the cost of finding
out that the queue is empty, paid once per question.
*/
const drainWait = 2 * time.Millisecond

/*
ErrSilent is an exchange that ran out of time with no matching reply.

Whose deadline it was does not matter to the device: it said nothing in the
time it was given. The error wraps context.DeadlineExceeded as well, so
errors.Is finds either. A driver that wants to know whether it was its own
caller who stopped waiting checks that caller's context, which is the one fact
this package cannot know.
*/
var ErrSilent = errors.New("no reply")

/*
ReportDevice is what Exchange speaks through: a *Handle, or a test's stand-in
for one. Write sends one output report; Read reads one input report, bounded
by the context, and reports a report that did not arrive in time as
context.DeadlineExceeded, as Handle.Read does.
*/
type ReportDevice interface {
	Write(report []byte) error
	Read(ctx context.Context, buf []byte) (int, error)
}

/*
Exchange writes req and returns the first report, read into a buffer of size
bytes, for which matches is true, skipping every other report until the
context's deadline.

The skipping is the point. A device that streams reports of its own accord
answers among them, not instead of them: after one Kraken status request
eleven of the next twelve reports were broadcasts, and one of them carries
status too, so it even looks right. matches is how a driver says which report
is the reply, and each driver's spec names its rule. There is no method that
writes and then simply reads the next report.

A context with no deadline is given two seconds for the whole exchange, so a
device that keeps talking about something else cannot hold it open forever.
Running out of time, on that bound or the caller's, is ErrSilent; a cancelled
context is the caller's error.
*/
func Exchange(ctx context.Context, dev ReportDevice, req []byte, matches func(reply []byte) bool, size int) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultWait)
		defer cancel()
	}
	if err := dev.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	for {
		n, err := dev.Read(ctx, buf)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: %w", ErrSilent, err)
			}
			return nil, err
		}
		if matches(buf[:n]) {
			out := make([]byte, n)
			copy(out, buf[:n])
			return out, nil
		}
	}
}
