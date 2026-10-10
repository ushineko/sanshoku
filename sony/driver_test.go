package sony

import (
	"context"
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
fake is a controller's interface. Over Bluetooth it sends the short report
until feature report 0x05 is read, and the long report after, as the
DualSense measured in spec 017 did. reports is what it streams in each mode;
silence costs nothing, the fake reports the deadline at once.
*/
type fake struct {
	bluetooth bool
	long      []byte
	silent    bool

	switched bool
	features [][]byte
	closed   bool
}

func (f *fake) GetFeature(_ context.Context, r []byte) error {
	f.features = append(f.features, append([]byte(nil), r...))
	if r[0] == calibration {
		f.switched = true
	}
	return nil
}

func (f *fake) Read(ctx context.Context, buf []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if f.silent {
		return 0, fmt.Errorf("no report from the fake: %w", context.DeadlineExceeded)
	}
	if f.bluetooth && !f.switched {
		short := make([]byte, 78)
		short[0] = usbReport
		return copy(buf, short), nil
	}
	return copy(buf, f.long), nil
}

func (f *fake) Drain(int)    {}
func (f *fake) Close() error { f.closed = true; return nil }

// open opens a candidate for product over the fake.
func open(t *testing.T, f *fake, product uint16) sanshoku.Device {
	t.Helper()
	bus := uint16(hidraw.BusUSB)
	if f.bluetooth {
		bus = hidraw.BusBluetooth
	}
	n := hidraw.Node{Path: "fake", Vendor: vendor, Product: product, Bus: bus, Name: "Sony Interactive Entertainment Something"}
	c := candidate(n, 50*time.Millisecond, func(string) (node, error) { return f, nil })
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	return dev
}

// Over Bluetooth the driver reads feature report 0x05 once, which is what
// turns the long report on, and reads the battery from the report after.
func TestBluetoothAsksForTheLongReportOnce(t *testing.T) {
	f := &fake{bluetooth: true, long: longReport(true, 0x07)}
	dev := open(t, f, 0x0CE6)

	for range 2 {
		got, err := dev.(battery.Source).Batteries(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, 75, got[0].Level)
		assert.Equal(t, "DualSense Wireless Controller", got[0].Name, "named by the allow-list, not the HID name")
		assert.Equal(t, battery.KindGamepad, got[0].Kind)
	}
	require.Len(t, f.features, 1, "the switch was asked for more than once on one handle")
	assert.Equal(t, byte(calibration), f.features[0][0])
	assert.Len(t, f.features[0], calibrationLen)
}

// Over USB the long report is the only report, and nothing is asked for.
func TestUSBAsksForNothing(t *testing.T) {
	f := &fake{long: longReport(false, 0x14)}
	dev := open(t, f, 0x0CE6)

	got, err := dev.(battery.Source).Batteries(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, battery.Charging, got[0].State)
	assert.Empty(t, f.features, "a USB controller was sent a request")
}

// A controller that sends nothing is no reading and no error, and over
// Bluetooth the switch is asked for again next time, in case it came back in
// its short mode.
func TestSilenceIsNoReadingAndTheSwitchIsAskedAgain(t *testing.T) {
	f := &fake{bluetooth: true, silent: true}
	dev := open(t, f, 0x0CE6)

	for range 2 {
		got, err := dev.(battery.Source).Batteries(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	}
	assert.Len(t, f.features, 2)
}

// A Sony product not on the allow-list is a candidate and is never opened.
func TestAProductOffTheListIsNotOpened(t *testing.T) {
	opened := false
	n := hidraw.Node{Path: "fake", Vendor: vendor, Product: 0x0F8A}
	c := candidate(n, time.Second, func(string) (node, error) { opened = true; return &fake{}, nil })
	_, err := c.Open(context.Background())
	require.ErrorIs(t, err, sanshoku.ErrUnsupported)
	assert.False(t, opened)
}
