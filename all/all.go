package all

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hwmon"
	"github.com/ushineko/sanshoku/support"
)

// Drivers is every driver in the module, in the order the specs landed them.
func Drivers() []sanshoku.Driver {
	return []sanshoku.Driver{
		hwmon.Driver{},
	}
}

// Support is every driver's support entries, in driver order.
func Support() []support.Entry {
	var out []support.Entry
	out = append(out, hwmon.Support()...)
	return out
}
