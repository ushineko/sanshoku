package all_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/sanshoku/all"
)

// The support table and the driver list describe the same module: every
// driver has at least one entry, and every entry names a driver that exists.
// docs/devices.md is generated from the table, so this is what keeps the page
// honest about what is in the module.
func TestEveryDriverHasSupportEntriesAndEveryEntryHasADriver(t *testing.T) {
	drivers := map[string]bool{}
	for _, d := range all.Drivers() {
		drivers[d.Name()] = true
	}
	covered := map[string]bool{}
	for _, e := range all.Support() {
		assert.True(t, drivers[e.Driver], "entry %q names driver %q, which all.Drivers does not have", e.Device, e.Driver)
		covered[e.Driver] = true
	}
	for name := range drivers {
		assert.True(t, covered[name], "driver %q has no entry in all.Support", name)
	}
}
