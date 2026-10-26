package aula

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
What a keyboard says it is. The receiver's product id is its chip's, and the
cable's is its chip's too: the F99 and the Womier L65 enumerate as the same
two devices with other layouts. So each is asked its model before a frame is
drawn on it, and only an F75 is drawn on.

The receiver's question is command 0x05 with an empty body, and the model is
the tenth byte of the answer; measured, `13 05 01 00 0a 03 00 00 00 00 cd 01`.
0xCD is the F75 and 0xA4 the F99 (OpenRGB issue 5166).
*/
const (
	commandModel = 0x05
	modelAt      = 10
	modelF75     = 0xCD
)

/*
frameFloor is the shortest interval at which frames are sent.

Measured (spec 019 E3). Through the receiver a frame at the message limit
takes 50 ms to write, 65 ms at the 95th percentile; a moving one looked even
at 80 ms and at 120 ms and rough at 60 ms. On the cable a frame is a 66 ms
control transfer whatever is in it, and a moving one looked even at 80 ms.
*/
const frameFloor = 80 * time.Millisecond

/*
frameHold is how long a frame is shown before the firmware's own effect is
back.

Measured (spec 019 E4). Through the receiver a colour sent again every second
stayed solid and one sent every 1.5 s flickered. On the cable both stayed
solid. Half a second is under the shorter of the two by a factor of two, which
leaves room for a frame that is late or lost on the radio.
*/
const frameHold = 500 * time.Millisecond

// model is what a keyboard answered when asked what it is.
type model struct {
	// asked is whether an answer was read. Silence is not an answer: a
	// keyboard that is asleep is asked again at the next frame.
	asked bool
	id    byte
}

// refuse is the error for a keyboard that answered and is not an F75.
func (m model) refuse() error {
	if m.asked && m.id != modelF75 {
		return fmt.Errorf("model %#02x is not an F75: %w", m.id, lighting.ErrNoCanvas)
	}
	return nil
}

// merged is held with px drawn over it. A pixel on a slot with no light is an
// error and held is unchanged: on the receiver every slot costs a byte of a
// message that has few to spare.
func merged(held frame, px []lighting.Pixel) (frame, error) {
	for _, p := range px {
		if int(p.ID) >= slotCount || !isKey[p.ID] {
			return held, fmt.Errorf("slot %d is not a light on the F75", p.ID)
		}
		held[p.ID] = colour{p.R, p.G, p.B}
	}
	return held, nil
}

// f75KeyList is the F75's lights, a copy the caller may keep.
func f75KeyList() []lighting.Key {
	return append([]lighting.Key(nil), f75Keys[:]...)
}

// Keys is the F75's 80 lights in slot order.
func (d *device) Keys() []lighting.Key { return f75KeyList() }

// Floor is the shortest interval at which frames are sent: 80 ms.
func (d *device) Floor() time.Duration { return frameFloor }

// Hold is how long a frame lasts before the firmware's effect returns. It
// makes the F75 a lighting.Fading.
func (d *device) Hold() time.Duration { return frameHold }

/*
Frame draws px over what the keyboard is showing and sends the whole board.

The keyboard darkens every key a message does not name, so the driver keeps
the frame and sends all of it each time; a light px does not name is left as
it was. A frame with more colours than a message holds is shown with its
nearest colours merged (ReceiverColours).

Nothing answers a frame. A keyboard that is asleep, out of range or switched
to its cable takes the write and shows nothing, and this returns nil: the
battery is how a consumer asks whether the keyboard is there. The first frame
on a handle asks the keyboard its model, and a keyboard that is not an F75 is
lighting.ErrNoCanvas. A keyboard that does not answer that question is
hidraw.ErrSilent, and is asked again at the next frame.
*/
func (d *device) Frame(ctx context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("lighting %s: %w", d.id.Path, os.ErrClosed)
	}
	next, err := merged(d.held, px)
	if err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	if err := d.askModel(ctx); err != nil {
		return fmt.Errorf("a frame to %s: %w", d.id, err)
	}
	d.held = next
	for _, p := range receiverFrame(&d.held) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
		if err := d.rd.Write(p); err != nil {
			return fmt.Errorf("a frame to %s: %w", d.id, err)
		}
	}
	return nil
}

// askModel asks the receiver's keyboard what it is, once per handle, and
// refuses one that is not an F75.
func (d *device) askModel(ctx context.Context) error {
	if err := d.knowModel(ctx); err != nil {
		return err
	}
	return d.model.refuse()
}

// knowModel asks the receiver's keyboard what it is, once per handle. Silence
// is an error and is asked again next time.
func (d *device) knowModel(ctx context.Context) error {
	if d.model.asked {
		return nil
	}
	actx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	d.rd.Drain(hidraw.QueueDepth)
	reply, err := hidraw.Exchange(actx, d.rd, packet(commandModel, 1, 0, 0, nil), answersTo(commandModel), reportLen)
	if err != nil {
		return fmt.Errorf("asking its model: %w", err)
	}
	d.model = model{asked: true, id: reply[modelAt]}
	return nil
}

/*
Release forgets the frame and sends nothing.

The firmware takes its lights back by itself when frames stop, so there is
nothing to send, and the keyboard is not rebooted as the Apex is. A frame
after a release starts from a dark board.
*/
func (d *device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rd == nil {
		return fmt.Errorf("releasing %s: %w", d.id.Path, os.ErrClosed)
	}
	d.held = frame{}
	return nil
}

// errNoAnswer is a wired keyboard whose answer is not the one asked for.
var errNoAnswer = errors.New("not an answer to the question")
