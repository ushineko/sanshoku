package nzxt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/hidraw"
)

// statusReply is a `75 01` report with the fields at liquidctl's offsets.
func statusReply(fields map[int]byte) []byte {
	r := make([]byte, reportLen)
	r[0], r[1] = 0x75, 0x01
	for i, v := range fields {
		r[i] = v
	}
	return r
}

/*
The decoder, from the offsets hotaru checked against liquidctl.

37.5 degrees is whole and tenths in 15 and 16; 2608 rpm is 0x0A30 little-endian
in 17 and 18. `FF FF` is liquidctl#172's firmware fault and not 255.5 degrees.
A `75 02` broadcast carries the same fields and is not the reply; decoding it
as one is the mistake the prefix match exists to prevent.
*/
func TestDecodeStatus(t *testing.T) {
	good := statusReply(map[int]byte{15: 37, 16: 5, 17: 0x30, 18: 0x0A, 19: 81, 23: 0xA6, 24: 0x04, 25: 51})
	s, err := DecodeStatus(good)
	require.NoError(t, err)
	assert.InDelta(t, 37.5, s.Coolant, 0.001)
	assert.Equal(t, 2608, s.PumpRPM)
	assert.Equal(t, 81, s.PumpDuty)
	assert.Equal(t, 1190, s.FanRPM)
	assert.Equal(t, 51, s.FanDuty)
	assert.True(t, s.HasPump)
	assert.True(t, s.HasFan)

	broadcast := append([]byte(nil), good...)
	broadcast[1] = 0x02

	for name, reply := range map[string][]byte{
		"a firmware fault":   statusReply(map[int]byte{15: 0xFF, 16: 0xFF, 17: 0x30, 18: 0x0A}),
		"a broadcast":        broadcast,
		"a truncated report": good[:20],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeStatus(reply)
			require.Error(t, err)
		})
	}
}

/*
The reply is read, not the broadcast.

The device streams `75 02` reports unasked: eleven of them arrived in the
twelve reads after one request on hotaru's machine. A reader matching only the
first byte parses a broadcast and calls it a reply, and because the broadcast
carries status too, it even looks right. The fake chatters for the same reason.
*/
func TestStatusIsReadFromTheReplyNotTheBroadcast(t *testing.T) {
	f := newFake()
	f.Chatter = 11
	f.Coolant = 41.2 // the broadcast carries the same number, so the prefix is what is tested

	s, err := withFake(f).Status(context.Background())

	require.NoError(t, err)
	assert.InDelta(t, 41.2, s.Coolant, 0.05)
	assert.Equal(t, 2608, s.PumpRPM)
	assert.False(t, s.Taken.IsZero(), "a reading with no time on it cannot be judged stale")
	require.Len(t, f.Told, 1)
	assert.Equal(t, []byte{0x74, 0x01}, f.Told[0][:2], "the status request is the one liquidctl sends")
	assert.Len(t, f.Told[0], reportLen, "a 64-byte report with no report ID prepended")
}

/*
One stolen reply and a queue of chatter together still produce a reading.

OpenRGB holds the same hidraw node and a report it reads is a report this
driver does not; the reply simply does not arrive. Asking again is the whole
mitigation. The backlog is what forty seconds of idleness leaves, and the drain
before the second ask is what keeps it from exhausting the twelve reads.
*/
func TestAReadingSurvivesAStolenReplyAndAQueueOfChatter(t *testing.T) {
	f := newFake()
	f.LoseFirst = 1
	f.Queued = 40
	f.Chatter = 11

	s, err := withFake(f).Status(context.Background())

	require.NoError(t, err, "one lost reply ended the reading")
	assert.InDelta(t, 37.5, s.Coolant, 0.05)
	assert.Len(t, f.Told, 2, "the question was not asked again")
}

/*
A firmware fault is an error, and is not cached: the next call asks again.

A coolant temperature of 255.5 would raise an alarm about the coolant rather
than about the cooler, and a device that failed once may have recovered by the
next call.
*/
func TestAFirmwareFaultIsAnErrorThatIsNotCached(t *testing.T) {
	f := newFake()
	f.Faulty = true
	d := withFake(f)

	_, err := d.Status(context.Background())
	require.ErrorIs(t, err, errFault)

	f.mu.Lock()
	f.Faulty = false
	f.mu.Unlock()
	s, err := d.Status(context.Background())
	require.NoError(t, err, "the fault was cached")
	assert.InDelta(t, 37.5, s.Coolant, 0.05)
	assert.Len(t, f.Told, 2)
}

// Persistent silence is asked three times and then reported, naming the reply
// that did not come, and as silence.
func TestSilenceIsAskedThreeTimesAndSaysWhatWasTried(t *testing.T) {
	f := newFake()
	f.Silent = true

	_, err := withFake(f).Status(context.Background())

	require.ErrorIs(t, err, hidraw.ErrSilent)
	assert.ErrorContains(t, err, "7501")
	assert.Len(t, f.Told, exchanges, "it gave up without asking again")
}

// A question is a place a program waits, and a caller that has given up must
// not wait for a device that keeps talking about something else.
func TestAnAbandonedReadGivesUp(t *testing.T) {
	f := newFake()
	f.Chatter = 1000
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := withFake(f).Status(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Len(t, f.Told, 1, "a cancelled read was retried")
}

/*
Without the drain, a backlog does exhaust the twelve reads, so the test above
measures something real. hotaru's service reported this as "no 7501 reply in
12 reports", only after it had been left alone for a while.
*/
func TestNotDrainingTheBacklogIsWhatFailed(t *testing.T) {
	f := newFake()
	f.Queued = 40

	_, err := await(context.Background(), f, askStatus, askStatusB)

	require.ErrorIs(t, err, hidraw.ErrSilent)
	assert.ErrorContains(t, err, "12 reports")
}
