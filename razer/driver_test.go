package razer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

/*
fake is a Razer control node that is not one, carrying only the behaviours
spec 003 R3.1 lists and hayami measured:

  - It answers on one transaction ID and says not-supported on every other,
    as a device does on the IDs its model is not addressed by. OpenRazer picks
    the ID per model; the dock hayami measured answered some and not others.
  - With busy set, it says busy on every ID, as it does while the openrazer
    daemon is mid-exchange on the same node or the mouse has gone to sleep.

The settle time is zero here; the device's own need for it is not something a
fake can show.
*/
type fake struct {
	answers byte
	level   byte
	charge  byte
	busy    bool

	// tried is the transaction ID of every request, in order.
	tried   []byte
	pending []byte
}

func (f *fake) SetFeature(_ context.Context, req []byte) error {
	body := req[1:]
	transaction, command := body[1], body[7]
	f.tried = append(f.tried, transaction)
	switch {
	case f.busy:
		f.pending = reply(statusBusy, command, 0)
	case transaction != f.answers:
		f.pending = reply(statusNotSupported, command, 0)
	case command == commandBatteryLevel:
		f.pending = reply(statusOK, command, f.level)
	default:
		f.pending = reply(statusOK, command, f.charge)
	}
	return nil
}

func (f *fake) GetFeature(_ context.Context, out []byte) error {
	if f.pending == nil {
		return errors.New("nothing to read")
	}
	copy(out, f.pending)
	return nil
}

// dock is a Mouse Dock Pro speaking through f.
func dock(f *fake) *device {
	return &device{
		id:      sanshoku.Identity{Vendor: razerVendor, Product: productMouseDockPro, Name: "Razer Mouse Dock Pro"},
		timeout: defaultTimeout,
		fd:      f,
	}
}

/*
The transaction ID is found by trying, remembered, and kept through silence.

The first two IDs in the list say not-supported and the third answers, so the
search runs in order until one does. A second poll asks once, on the ID that
answered. Then the device goes busy: that is what a contended or sleeping
device says on the *right* ID, several times an hour, and it is no reading and
no error, and it does not send the driver searching the list again.
*/
func TestTheTransactionIDIsFoundRememberedAndKeptThroughBusy(t *testing.T) {
	answers := transactions[2]
	f := &fake{answers: answers, level: 0xFF, charge: 0x01}
	d := dock(f)

	found, err := d.Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 100, found[0].Level)
	assert.Equal(t, battery.Full, found[0].State, "charging at 100 is full")
	assert.Equal(t, battery.KindMouse, found[0].Kind)
	assert.Equal(t, "Razer Mouse Dock Pro", found[0].Name, "the battery is named after the node")
	assert.Equal(t, transactions[:3], f.tried[:3], "the list is tried in order until one answers")

	f.tried = nil
	_, err = d.Batteries(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []byte{answers, answers}, f.tried, "the search ran again on a device that had answered")

	f.busy, f.tried = true, nil
	found, err = d.Batteries(context.Background())
	require.NoError(t, err, "busy was reported as an error")
	assert.Empty(t, found)
	assert.Equal(t, []byte{answers}, f.tried, "a busy device made the driver search the list again")
}
