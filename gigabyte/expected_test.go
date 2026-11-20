package gigabyte

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lighting"
)

/*
Every layout in the tables is one the driver can draw: zone names unique, at
most one header on each slot, single-colour lights on distinct registers below
8, and no board both measured and expected (sanshoku #111).
*/
func TestEveryLayoutIsOneTheDriverCanDraw(t *testing.T) {
	check := func(k boardKey, layout []zoneDef) {
		names, slots, lights := map[string]bool{}, map[int]bool{}, map[byte]bool{}
		for _, z := range layout {
			assert.False(t, names[z.name], "%v: zone %q twice", k, z.name)
			names[z.name] = true
			if z.header {
				assert.False(t, slots[z.slot], "%v: two headers on slot %d", k, z.slot)
				slots[z.slot] = true
				continue
			}
			assert.Less(t, z.light, byte(8), "%v: %q", k, z.name)
			assert.False(t, lights[z.light], "%v: two lights on register %d", k, z.light)
			lights[z.light] = true
		}
		assert.NotEmpty(t, layout, "%v", k)
	}
	for k, l := range boards {
		check(k, l)
	}
	for k, l := range expected {
		check(k, l)
		_, measured := boards[k]
		assert.False(t, measured, "%v is both measured and expected", k)
	}
	for _, b := range expectedTwice {
		for k := range expected {
			assert.NotEqual(t, b, k.board, "%q is listed twice by OpenRGB and must not be expected", b)
		}
	}
}

// An expected board opens with its layout, named as OpenRGB names it.
func TestAnExpectedBoardOpensWithItsLayout(t *testing.T) {
	f := newFake(0)
	c := candidate(masterXNode, func(string) (node, error) { return f, nil }, "B650 AORUS ELITE AX")
	dev, err := c.Open(context.Background())
	require.NoError(t, err)
	d := dev.(*device)
	assert.Equal(t, "B650 AORUS ELITE AX (0x5701ABCD)", d.Identity().Name)
	assert.Equal(t, []lighting.Zone{
		{Name: "D_LED1", First: 0, Max: 256},
		{Name: "D_LED2", First: 0, Max: 256},
		{Name: "LED_C1", First: 0, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "LED_CPU", First: 1, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "Chipset Accent", First: 2, Count: 1, Shape: lighting.ShapeSingle},
		{Name: "LED_C2", First: 3, Count: 1, Shape: lighting.ShapeSingle},
	}, d.Zones())
}

// A board OpenRGB lists twice with different layouts is refused after its
// report, like any board not in the tables.
func TestABoardListedTwiceIsRefused(t *testing.T) {
	require.NotEmpty(t, expectedTwice)
	f := newFake(0)
	c := candidate(masterXNode, func(string) (node, error) { return f, nil }, expectedTwice[0])
	_, err := c.Open(context.Background())
	require.Error(t, err)
	assert.Equal(t, []string{"60"}, f.told(1))
}
