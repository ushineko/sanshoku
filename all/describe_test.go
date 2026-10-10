package all_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/support"
)

/*
Spec 014. Every driver describes itself, and its description says what its
support entries say: the same capabilities, and Windows among its platforms
exactly when an entry stands there above Listed. A description that drifted
from the table would have a consumer ask a driver on a system it does not
read, or skip one that does.
*/
func TestEveryDriverDescribesItselfAsItsSupportEntriesDo(t *testing.T) {
	entries := all.Support()
	for _, d := range all.Drivers() {
		desc, ok := sanshoku.Describe(d)
		require.True(t, ok, "driver %q gives no Description", d.Name())
		assert.NotEmpty(t, desc.Name, "driver %q has no Name", d.Name())
		assert.NotEmpty(t, desc.Finds, "driver %q does not say what it finds", d.Name())

		var caps []string
		windows := false
		for _, e := range entries {
			if e.Driver != d.Name() {
				continue
			}
			for _, c := range e.Capabilities {
				if !slices.Contains(caps, c) {
					caps = append(caps, c)
				}
			}
			if e.Windows != nil && e.Windows.Tier != support.Listed {
				windows = true
			}
		}
		assert.ElementsMatch(t, caps, desc.Capabilities, "driver %q: capabilities differ from its support entries", d.Name())
		assert.True(t, desc.On("linux"), "driver %q: every driver reads on Linux, where its entries' tiers are", d.Name())
		assert.Equal(t, windows, desc.On("windows"), "driver %q: Windows in its platforms and in its support entries disagree", d.Name())
	}
}

// What a consumer says when a battery driver finds nothing, built from the
// descriptions: the lines hayami kept in a table of its own before spec 014.
func TestTheBatteryDriversSayWhatNothingFoundIs(t *testing.T) {
	var said []string
	for _, d := range all.Drivers() {
		desc, _ := sanshoku.Describe(d)
		if !desc.Offers("battery") {
			continue
		}
		line := "no " + desc.Name + " " + desc.Finds
		if !slices.Contains(said, line) {
			said = append(said, line)
		}
	}
	assert.Equal(t, []string{
		"no Logitech receiver",
		"no Razer device",
		"no SteelSeries device",
		"no Bluetooth device with a battery",
		"no AULA receiver",
		"no Sony controller",
	}, said)
}
