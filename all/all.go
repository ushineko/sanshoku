package all

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/apple"
	"github.com/ushineko/sanshoku/asus"
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/bluez"
	"github.com/ushineko/sanshoku/corsair"
	"github.com/ushineko/sanshoku/corsairdram"
	"github.com/ushineko/sanshoku/evga"
	"github.com/ushineko/sanshoku/gigabyte"
	"github.com/ushineko/sanshoku/hwmon"
	"github.com/ushineko/sanshoku/keychron"
	"github.com/ushineko/sanshoku/logitech"
	"github.com/ushineko/sanshoku/msi"
	"github.com/ushineko/sanshoku/nzxt"
	"github.com/ushineko/sanshoku/powersupply"
	"github.com/ushineko/sanshoku/razer"
	"github.com/ushineko/sanshoku/sony"
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
		bluez.Driver{},
		apple.Driver{},
		aula.Driver{},
		corsair.Driver{},
		asus.Driver{},
		keychron.Driver{},
		msi.Driver{},
		sony.Driver{},
		powersupply.Driver{},
		gigabyte.Driver{},
		msi.Board{},
		evga.Driver{},
		corsairdram.Driver{},
		razer.Lights{},
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
	out = append(out, bluez.Support()...)
	out = append(out, apple.Support()...)
	out = append(out, aula.Support()...)
	out = append(out, corsair.Support()...)
	out = append(out, asus.Support()...)
	out = append(out, keychron.Support()...)
	out = append(out, msi.Support()...)
	out = append(out, sony.Support()...)
	out = append(out, powersupply.Support()...)
	out = append(out, gigabyte.Support()...)
	out = append(out, msi.BoardSupport()...)
	out = append(out, evga.Support()...)
	out = append(out, corsairdram.Support()...)
	out = append(out, razer.LightsSupport()...)
	return out
}
