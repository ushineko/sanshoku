package steelseries

import (
	"time"

	"github.com/ushineko/sanshoku/support"
)

// steelseriesMatch is the rule Find matches by, in words.
const steelseriesMatch = "vendor 1038, usage page 0xFFC0"

/*
Support is the steelseries driver's support table, one entry per product in
the allow-list.

The Apex Pro TKL Wireless Gen 3 is Tested: the bench on the machine that has
it read 100% full on its cable on 2026-09-29, and on 2026-10-01 streamed
frames to it on its cable and through its receiver and read its battery on
both (spec 010). It is the one product with a lighting canvas. The Aerox and Prime mice are rivalcfg's 0x92 family and Expected.
The Rival family speaks rivalcfg's 0xAA protocol, which is not implemented, and
is Listed. The Arctis Nova Pro Wireless is Tested on its X base station
(1038:12e5), which the bench read at 75% on 2026-09-29 in agreement with
headsetcontrol; the other base station, 1038:12e0, shares the entry by
HeadsetControl's protocol table. A SteelSeries product in none of these is
found and refused.
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
	apex.Tier = support.Tested
	apex.Hardware = "SteelSeries Apex Pro TKL Wireless Gen 3, on its cable (1038:1646) and its receiver (1038:1644)"
	apex.Tested = lightingBenched
	apex.Capabilities = []string{"battery", "lighting"}
	apex.Notes = "0x92 wired, 0xD2 through the dongle. " +
		"Lighting: 0x61 direct frames of 85 keys, as SteelSeries GG streams them; floor 16 ms; 0x41 releases by rebooting the keyboard"

	arctis := support.Entry{
		Driver:       driverName,
		Device:       "Arctis Nova Pro Wireless",
		Match:        steelseriesMatch + "; 1038:12e5 (X base station), or 1038:12e0 (base station, expected)",
		Vendor:       steelseriesVendor,
		Products:     []uint16{0x12E0, 0x12E5},
		Capabilities: []string{"battery"},
		Tier:         support.Tested,
		Hardware:     "SteelSeries Arctis Nova Pro Wireless, base station 1038:12e5",
		Tested:       novaProBenched,
		Spec:         6,
		Notes: "HeadsetControl's 06 b0 battery report through the base station; " +
			"12e0 is by protocol, not measured",
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

// lightingBenched is the date of the spec 010 bench runs on the Apex: frames
// through its receiver and on its cable, and the battery on both.
var lightingBenched = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

// novaProBenched is the date of the bench run against the Nova Pro Wireless X
// base station (spec 006).
var novaProBenched = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)
