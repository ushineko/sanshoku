package battery

/*
Band is a battery as a device without a gauge reports it.

Four values and no more: it is the device's own resolution, and the shape its
indicator uses. A Logitech K800 reads its HID++ 1.0 register 0x07 this way.
*/
type Band int

// The bands, low to high. BandUnknown is the zero value and is not a reading.
const (
	BandUnknown Band = iota
	BandCritical
	BandLow
	BandGood
	BandFull
)

// Segments is how many of four are filled.
func (b Band) Segments() int {
	switch b {
	case BandCritical:
		return 1
	case BandLow:
		return 2
	case BandGood:
		return 3
	case BandFull:
		return 4
	default:
		return 0
	}
}

// String is the band's word, empty for BandUnknown.
func (b Band) String() string {
	switch b {
	case BandCritical:
		return "Critical"
	case BandLow:
		return "Low"
	case BandGood:
		return "Good"
	case BandFull:
		return "Full"
	default:
		return ""
	}
}
