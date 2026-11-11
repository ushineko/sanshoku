package sanshoku_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
)

// fallbackDriver is fakeDriver marked as a Fallback, as powersupply is.
type fallbackDriver struct{ fakeDriver }

func (fallbackDriver) Fallback() {}

func keyed(name, key string) sanshoku.Candidate {
	return sanshoku.Candidate{Identity: sanshoku.Identity{Name: name}, Driver: "fake", Key: key}
}

func names(found []sanshoku.Candidate) []string {
	var out []string
	for _, c := range found {
		out = append(out, c.Name)
	}
	return out
}

// The device a vendor driver reads is listed once, by that driver; what the
// fallback found on other devices is kept (spec 022).
func TestAFallbackGivesWayToADriverThatFoundTheSameDevice(t *testing.T) {
	found, err := sanshoku.Scan(context.Background(),
		fakeDriver{name: "sony", found: []sanshoku.Candidate{keyed("DualSense", "bt:aa:bb:cc:dd:ee:01")}},
		fallbackDriver{fakeDriver{name: "kernel", found: []sanshoku.Candidate{
			keyed("kernel DualSense", "bt:aa:bb:cc:dd:ee:01"),
			keyed("Pro Controller", "bt:aa:bb:cc:dd:ee:02"),
			keyed("no key", ""),
		}}},
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"DualSense", "Pro Controller", "no key"}, names(found))
}

// The order the drivers run in does not decide it.
func TestAFallbackGivesWayWhicheverRunsFirst(t *testing.T) {
	found, _ := sanshoku.Scan(context.Background(),
		fallbackDriver{fakeDriver{name: "kernel", found: []sanshoku.Candidate{keyed("kernel", "usb:1-4")}}},
		fakeDriver{name: "steelseries", found: []sanshoku.Candidate{keyed("Arctis", "usb:1-4")}},
	)

	assert.Equal(t, []string{"Arctis"}, names(found))
}

// Only a fallback gives way: two direct drivers on one device are both kept,
// and with no direct driver the fallback keeps everything.
func TestOnlyAFallbackIsDropped(t *testing.T) {
	found, _ := sanshoku.Scan(context.Background(),
		fakeDriver{name: "a", found: []sanshoku.Candidate{keyed("a", "usb:1-4")}},
		fakeDriver{name: "b", found: []sanshoku.Candidate{keyed("b", "usb:1-4")}},
	)
	assert.Equal(t, []string{"a", "b"}, names(found))

	found, _ = sanshoku.Scan(context.Background(),
		fallbackDriver{fakeDriver{name: "kernel", found: []sanshoku.Candidate{keyed("x", "usb:1-4"), keyed("y", "usb:1-4")}}},
	)
	assert.Equal(t, []string{"x", "y"}, names(found))
}

// A direct candidate with no key drops nothing, even a fallback with no key.
func TestNoKeyDropsNothing(t *testing.T) {
	found, _ := sanshoku.Scan(context.Background(),
		fakeDriver{name: "a", found: []sanshoku.Candidate{keyed("a", "")}},
		fallbackDriver{fakeDriver{name: "kernel", found: []sanshoku.Candidate{keyed("k", "")}}},
	)
	assert.Equal(t, []string{"a", "k"}, names(found))
}
