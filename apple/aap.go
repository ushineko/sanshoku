package apple

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/ushineko/sanshoku/battery"
)

/*
Apple's accessory protocol, as far as a battery reading needs it.

AirPods report nothing through BlueZ: there is no org.bluez.Battery1 on the
device, because the code that would put one there — BlueZ's battery provider,
reading Apple's HFP AT+IPHONEACCEV — is behind the Experimental setting, which
is off by default. A reading that needed a line added to a system configuration
file would be missing on every machine nobody has prepared.

So the device is asked directly, over an L2CAP channel, the way LibrePods and
the program hayami succeeded both do it. The exchange is: connect, say hello,
set features, ask for notifications, and the device starts reporting. The
constants below are not documented by anyone; they are what one firmware
answers to, on the AirPods Pro hayami measured (hayami spec 009).
*/

// aapPSM is the L2CAP port the accessory protocol listens on.
const aapPSM = 0x1001

// The exchange, in order. Each is sent and the next is sent when the one
// before it is acknowledged.
var (
	aapHandshake     = mustDecode("00000400010002000000000000000000")
	aapSetFeatures   = mustDecode("040004004d00d700000000000000")
	aapNotifications = mustDecode("040004000f00ffffffffff")
)

// What the device says back.
var (
	aapHandshakeAck = mustDecode("01000400")
	aapFeaturesAck  = mustDecode("040004002b00")
	aapBattery      = mustDecode("040004000400")
)

func mustDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("bad constant: " + s)
	}
	return b
}

// The cells a device can report, as the protocol numbers them.
const (
	aapHeadset = 0x01
	aapRight   = 0x02
	aapLeft    = 0x04
	aapCase    = 0x08
)

// What a cell says it is doing. 0x02 is draining, which is the ordinary case
// and needs no name here.
const (
	aapCharging  = 0x01
	aapNotOnBody = 0x04
)

// recordSize is one cell's record: the cell, a constant, the level, what it
// is doing, and another constant.
const recordSize = 5

/*
ErrNoBatteryPacket is a device that connected and never reported, or reported
no cell.

Not a failure of the machine or of this code: a firmware that does not answer
this exchange is a device this build cannot read. It is silence, and a
device's Batteries swallows it into no reading.
*/
var ErrNoBatteryPacket = errors.New("the device did not report a battery")

// channel is an open accessory-protocol conversation: an *l2cap.Conn, or a
// test's stand-in for one.
type channel interface {
	Send([]byte) error
	Receive(context.Context) ([]byte, error)
	Close() error
}

/*
DecodeBattery reads an accessory-protocol battery packet into its cells, in
the order the device sent them.

The layout is a six-byte prefix, a count, and then that many five-byte records:
the cell (0x01 headset, 0x02 right, 0x04 left, 0x08 case), a constant, the
level, what it is doing (0x01 charging, 0x02 draining, 0x04 not on body), and
another constant.

**A cell that is not there says so; it does not go missing.** A case left on
the desk is reported at level zero with the "not on body" status, and a reader
that took the level at face value would draw a flat battery for a case that is
simply elsewhere. Those records are dropped rather than read, as is a level
above 100 and a cell number this build does not know.

A packet that is not a battery packet, or promises more records than it
carries, is an error rather than a partial reading.
*/
func DecodeBattery(packet []byte) ([]battery.Cell, error) {
	if len(packet) < len(aapBattery)+1 || string(packet[:len(aapBattery)]) != string(aapBattery) {
		return nil, errors.New("not an accessory-protocol battery packet")
	}

	count := int(packet[len(aapBattery)])
	body := packet[len(aapBattery)+1:]
	if len(body) < count*recordSize {
		return nil, fmt.Errorf("a battery packet promising %d cells and carrying %d bytes", count, len(body))
	}

	var out []battery.Cell
	for i := range count {
		rec := body[i*recordSize : (i+1)*recordSize]

		cell, ok := cellOf(rec[0])
		if !ok {
			// A cell this build has never heard of. Skipped rather than
			// guessed at, and not an error: a future device with a fourth
			// earbud is not a broken packet.
			continue
		}
		if rec[3] == aapNotOnBody {
			continue
		}
		if rec[2] > 100 {
			continue
		}

		out = append(out, battery.Cell{
			Cell:     cell,
			Level:    int(rec[2]),
			Charging: rec[3] == aapCharging,
		})
	}
	return out, nil
}

// cellOf names a cell number, reporting whether it is one this build knows.
func cellOf(b byte) (battery.CellKind, bool) {
	switch b {
	case aapHeadset:
		return battery.Headset, true
	case aapLeft:
		return battery.Left, true
	case aapRight:
		return battery.Right, true
	case aapCase:
		return battery.Case, true
	default:
		return 0, false
	}
}

/*
batteryOf turns a device's cells into one reading.

The **lower of the two ears**, because that is the one that will stop working
first and it is what somebody glancing at a panel wants to know. The case is
deliberately not part of it: a case at 5 % while the ears are full is not a
warning about anything the wearer is doing, and letting it set the number
would make a panel shout about a thing in a drawer.

A device reporting a single cell is that cell, which is what a set of
headphones that is one piece looks like.
*/
func batteryOf(name string, cells []battery.Cell) (battery.Battery, error) {
	if len(cells) == 0 {
		return battery.Battery{}, ErrNoBatteryPacket
	}

	// In a fixed order, not the order the device happened to send them. The
	// AirPods hayami was written against report right before left, and a
	// panel that drew "R 100  L 100" makes a reader parse the labels instead
	// of the numbers. battery.CellKind is declared in the order to draw.
	sorted := append([]battery.Cell(nil), cells...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Cell < sorted[j].Cell })

	b := battery.Battery{Name: name, Cells: sorted}

	level, has := -1, false
	for _, c := range sorted {
		if c.Cell == battery.Case {
			continue
		}
		if !has || c.Level < level {
			level, has = c.Level, true
		}
	}
	if !has {
		// Only the case answered. It is a reading, and it is the only one
		// there is, so it is reported rather than dropped.
		level = sorted[0].Level
	}
	b.Level, b.HasLevel = level, true

	if charging(sorted) {
		b.State = battery.Charging
	}
	return b, nil
}

// charging reports whether any cell that is not the case is on the cable.
func charging(cells []battery.Cell) bool {
	for _, c := range cells {
		if c.Cell != battery.Case && c.Charging {
			return true
		}
	}
	return false
}

/*
readAAP runs the exchange on c and returns the first battery packet's cells.

The device is talked through three steps and then reports on its own. Packets
that are neither an acknowledgement nor a battery are the device's ordinary
chatter — it has a great deal to say about its case, its firmware and its
serial numbers — and are waited through. Nothing by the context's deadline is
ErrNoBatteryPacket.
*/
func readAAP(ctx context.Context, c channel) ([]battery.Cell, error) {
	if err := c.Send(aapHandshake); err != nil {
		return nil, err
	}

	sentFeatures, sentNotifications := false, false
	for {
		pkt, err := c.Receive(ctx)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrNoBatteryPacket
		}
		if err != nil {
			return nil, err
		}

		switch {
		case startsWith(pkt, aapBattery):
			return DecodeBattery(pkt)

		case startsWith(pkt, aapHandshakeAck) && !sentFeatures:
			if err := c.Send(aapSetFeatures); err != nil {
				return nil, err
			}
			sentFeatures = true

		case startsWith(pkt, aapFeaturesAck) && !sentNotifications:
			if err := c.Send(aapNotifications); err != nil {
				return nil, err
			}
			sentNotifications = true
		}
	}
}

// startsWith reports whether a packet opens with a prefix.
func startsWith(pkt, prefix []byte) bool {
	return len(pkt) >= len(prefix) && string(pkt[:len(prefix)]) == string(prefix)
}
