package apple

import "github.com/ushineko/sanshoku/support"

/*
Support is the apple driver's support table.

One Expected entry for every Apple audio accessory, because every one goes
through the same exchange. The accessory protocol was measured by hayami on a
pair of AirPods Pro (hayami spec 009); no AirPods were connected when spec
004's bench ran, so no generation is Tested yet. A bench run on a pair promotes
a Tested entry for that generation, named as the product reports it.
*/
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       driverName,
			Device:       "AirPods and other Apple audio accessories",
			Match:        "connected BlueZ device, Modalias vendor 004c, audio icon or audio sink profile; L2CAP PSM 0x1001",
			Vendor:       appleVendor,
			Capabilities: []string{"battery"},
			Tier:         support.Expected,
			Spec:         4,
			Notes: "per-ear and case levels over the accessory protocol, falling back to Battery1; " +
				"measured by hayami on AirPods Pro, not on spec 004's bench",
		},
	}
}
