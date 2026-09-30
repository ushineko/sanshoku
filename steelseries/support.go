package steelseries

import "github.com/ushineko/sanshoku/support"

// steelseriesMatch is the rule Find matches by, in words.
const steelseriesMatch = "vendor 1038, usage page 0xFFC0"

/*
Support is the steelseries driver's support table, one entry per product in
the allow-list.

The Apex Pro TKL Wireless Gen 3 was measured by hayami (spec 016) and was not
on the desk when spec 003's bench ran, so it is Expected until a bench run
promotes it. The Aerox and Prime mice are rivalcfg's 0x92 family and Expected.
The Rival family speaks rivalcfg's 0xAA protocol, which is not implemented, and
is Listed, as is the Arctis Nova Pro Wireless, the headset that shares the
usage page. A SteelSeries product in none of these is found and refused.
*/
func Support() []support.Entry {
	modernEntry := func(device, ids string, products ...uint16) support.Entry {
		return support.Entry{
			Driver:       driverName,
			Device:       device,
			Match:        steelseriesMatch + "; " + ids,
			Vendor:       steelseriesVendor,
			Products:     products,
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         3,
			Notes:        "rivalcfg's 0x92 battery command; not read by hayami or this module",
		}
	}
	legacyEntry := func(device, ids string, product uint16) support.Entry {
		return support.Entry{
			Driver:   driverName,
			Device:   device,
			Match:    steelseriesMatch + "; " + ids,
			Vendor:   steelseriesVendor,
			Products: []uint16{product},
			Tier:     support.Listed,
			Spec:     3,
			Notes: "rivalcfg's 0xAA 0x01 battery command, whose reply carries no command echo to match on; " +
				"not implemented without a device to measure",
		}
	}

	apex := modernEntry("Apex Pro TKL Wireless Gen 3", "1038:1644 (2.4 GHz) or 1038:1646 (cable)", 0x1644, 0x1646)
	apex.Notes = "0x92 wired, 0xD2 through the dongle; measured by hayami, not on spec 003's bench"

	arctis := support.Entry{
		Driver:   driverName,
		Device:   "Arctis Nova Pro Wireless",
		Match:    steelseriesMatch + "; 1038:12e5",
		Vendor:   steelseriesVendor,
		Products: []uint16{0x12E5},
		Tier:     support.Listed,
		Spec:     3,
		Notes: "a headset on the same usage page as the keyboards; found, refused, never written to. " +
			"Its battery is headsetcontrol's today and a candidate spec (docs/devices.md)",
	}

	return []support.Entry{
		apex,
		modernEntry("Aerox 3 Wireless", "1038:1838 or 1038:183a", 0x1838, 0x183A),
		modernEntry("Aerox 5 Wireless", "1038:1852 or 1038:1854", 0x1852, 0x1854),
		modernEntry("Aerox 9 Wireless", "1038:1858 or 1038:185a", 0x1858, 0x185A),
		modernEntry("Prime Wireless", "1038:1840 or 1038:1842", 0x1840, 0x1842),
		legacyEntry("Rival 3 Wireless", "1038:1830", 0x1830),
		legacyEntry("Rival 3 Wireless Gen 2", "1038:1872", 0x1872),
		legacyEntry("Rival 650 Wireless", "1038:172b", 0x172B),
		arctis,
	}
}
