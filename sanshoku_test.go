package sanshoku_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
)

// fakeDriver is the one fake the root API is tested with: a driver that finds
// what it is told to and fails the way it is told to. It opens nothing.
type fakeDriver struct {
	name  string
	found []sanshoku.Candidate
	err   error
}

func (d fakeDriver) Name() string { return d.name }

func (d fakeDriver) Find(context.Context) ([]sanshoku.Candidate, error) { return d.found, d.err }

// fakeBattery is a device that satisfies battery.Source and nothing else.
type fakeBattery struct{}

func (fakeBattery) Identity() sanshoku.Identity { return sanshoku.Identity{Name: "A Mouse"} }
func (fakeBattery) Close() error                { return nil }
func (fakeBattery) Batteries(context.Context) ([]battery.Battery, error) {
	return nil, nil
}

func candidates(names ...string) []sanshoku.Candidate {
	out := make([]sanshoku.Candidate, 0, len(names))
	for _, n := range names {
		out = append(out, sanshoku.Candidate{Identity: sanshoku.Identity{Name: n}, Driver: "fake"})
	}
	return out
}

// Absence is not an error: a machine with none of one driver's devices scans
// clean, with the other driver's candidates intact.
func TestAnAbsentDriverIsNotAnError(t *testing.T) {
	found, err := sanshoku.Scan(context.Background(),
		fakeDriver{name: "two", found: candidates("a", "b")},
		fakeDriver{name: "none", err: sanshoku.ErrAbsent},
	)

	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, "a", found[0].Name)
	assert.Equal(t, "b", found[1].Name)
}

// A driver that fails does not cost the candidates another found; the error
// comes back beside them.
func TestAFailingDriverKeepsTheOtherCandidatesAndReturnsItsError(t *testing.T) {
	broken := errors.New("the bus fell over")

	found, err := sanshoku.Scan(context.Background(),
		fakeDriver{name: "two", found: candidates("a", "b")},
		fakeDriver{name: "broken", err: broken},
	)

	require.Len(t, found, 2)
	require.ErrorIs(t, err, broken)
	assert.NotErrorIs(t, err, sanshoku.ErrAbsent)
}

// A device is asked what it can do by type assertion, and says so by name.
func TestCapabilitiesNamesWhatADeviceSatisfies(t *testing.T) {
	assert.Equal(t, []string{"battery"}, sanshoku.Capabilities(fakeBattery{}))
}
