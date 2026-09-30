package all

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hwmon"
	"github.com/ushineko/sanshoku/logitech"
	"github.com/ushineko/sanshoku/nzxt"
	"github.com/ushineko/sanshoku/razer"
	"github.com/ushineko/sanshoku/steelseries"
	"github.com/ushineko/sanshoku/support"
)

// Drivers is every driver in the module, in the order the specs landed them.
func Drivers() []sanshoku.Driver {
	return []sanshoku.Driver{
		hwmon.Driver{},
		logitech.Driver{},
		razer.Driver{},
		steelseries.Driver{},
		nzxt.Driver{},
	}
}

// Support is every driver's support entries, in driver order.
func Support() []support.Entry {
	var out []support.Entry
	out = append(out, hwmon.Support()...)
	out = append(out, logitech.Support()...)
	out = append(out, razer.Support()...)
	out = append(out, steelseries.Support()...)
	out = append(out, nzxt.Support()...)
	return out
}
