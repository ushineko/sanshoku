package sanshoku_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku"
)

// Two receivers on one desk are one Presence summed: their nodes and quiet
// slots added, their unreadable devices listed together, and the protocol
// those speak named once.
func TestPresencesAddUp(t *testing.T) {
	a := sanshoku.Presence{Nodes: 1, Quiet: 2}
	b := sanshoku.Presence{Nodes: 1, Quiet: 1, TooOld: []string{"Logitech K400"}, OldProtocol: "HID++ 1.0"}

	got := a.Add(b)

	assert.Equal(t, sanshoku.Presence{Nodes: 2, Quiet: 3, TooOld: []string{"Logitech K400"}, OldProtocol: "HID++ 1.0"}, got)
	assert.Empty(t, a.TooOld, "adding changed the receiver added to")
}

// A driver written outside this module gives no description, and says so.
func TestADriverWithoutADescriptionSaysSo(t *testing.T) {
	_, ok := sanshoku.Describe(bare{})
	assert.False(t, ok)
}

// bare is a driver with nothing but the interface.
type bare struct{}

func (bare) Name() string { return "bare" }

func (bare) Find(context.Context) ([]sanshoku.Candidate, error) { return nil, nil }
