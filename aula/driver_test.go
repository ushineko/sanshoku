package aula

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
)

/*
fake is the receiver's vendor interface. Each question is answered with the
replies queued for it, in order: nothing (asleep), the 0x0A status frame the
receiver pushes, a battery reply. Silence costs nothing: the fake reports the
deadline at once.
*/
type fake struct {
	answers  [][][]byte
	pending  [][]byte
	requests [][]byte
	drained  int
	closed   bool
}

func (f *fake) Write(req []byte) error {
	f.requests = append(f.requests, append([]byte(nil), req...))
	if len(f.answers) > 0 {
		f.pending = append(f.pending, f.answers[0]...)
		f.answers = f.answers[1:]
	}
	return nil
}

func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(f.pending) == 0 {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	r := f.pending[0]
	f.pending = f.pending[1:]
	return copy(buf, r), nil
}

func (f *fake) Drain(int)    { f.drained++ }
func (f *fake) Close() error { f.closed = true; return nil }

var (
	status    = []byte{0x13, 0x0a, 0x01, 0x00, 0x04, 0x05, 0x64, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x8c}
	onBattery = []byte{0x13, 0x4a, 0x01, 0x00, 0x02, 0x61, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xc2}
)

// receiver is the F75's receiver interface as Nodes reports it.
var receiver = hidraw.Node{
	Path: "receiver", Name: "Compx 2.4G Wireless Receiver", Vendor: receiverVendor, Product: 0xFA09,
}

// open opens the receiver over f.
func open(t *testing.T, f *fake) sanshoku.Device {
	t.Helper()
	c := candidate(receiver, 20*time.Millisecond, func(string) (node, error) { return f, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	dev.(*device).wait = time.Millisecond
	return dev
}

/*
A question the keyboard does not answer is asked once more, and the second
answer is the reading. Measured: the first question after the F75 was switched
to its receiver went unanswered, and the link was up moments later. The
status frame the receiver pushes ahead of the reply is skipped.
*/
func TestAnUnansweredQuestionIsAskedOnceMore(t *testing.T) {
	f := &fake{answers: [][][]byte{nil, {status, onBattery}}}
	dev := open(t, f)

	got, err := dev.(battery.Source).Batteries(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "F75", got[0].Name)
	assert.Equal(t, 97, got[0].Level)
	assert.Equal(t, battery.KindKeyboard, got[0].Kind)
	assert.Len(t, f.requests, 2)
	assert.Equal(t, request(), f.requests[0])
	assert.Equal(t, 2, f.drained, "the queue was not drained before each question")
}

// A keyboard that answers neither question is asleep or off: no reading and
// no error, and not a third question.
func TestAKeyboardThatAnswersNothingIsNoReadingAndNoError(t *testing.T) {
	f := &fake{}
	dev := open(t, f)

	got, err := dev.(battery.Source).Batteries(context.Background())

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Len(t, f.requests, 2)
}

// A product the allow-list does not name is found and never opened, let
// alone written to.
func TestAnUnknownProductIsFoundAndNeverOpened(t *testing.T) {
	n := receiver
	n.Product = 0x1234
	c := candidate(n, time.Millisecond, func(string) (node, error) {
		t.Fatal("an unknown product was opened")
		return nil, nil
	})

	_, err := c.Open(context.Background())

	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	assert.Equal(t, "Compx 2.4G Wireless Receiver", c.Name)
}

// A closed device reads nothing and says so.
func TestAClosedDeviceSaysSo(t *testing.T) {
	f := &fake{}
	dev := open(t, f)
	require.NoError(t, dev.Close())
	assert.True(t, f.closed)

	_, err := dev.(battery.Source).Batteries(context.Background())

	require.Error(t, err)
	assert.False(t, errors.Is(err, sanshoku.ErrGone))
}
