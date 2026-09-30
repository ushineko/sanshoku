package battery

import "context"

// Source is a device that can report its batteries.
//
// One device may report several readings: a receiver answers for every device
// paired to it. A device that is present and asleep returns no reading and no
// error; silence is a reading withheld, not a failure.
type Source interface {
	Batteries(context.Context) ([]Battery, error)
}

// State is what a battery is doing, as distinct from how full it is.
//
// It is kept apart from the level because the two answer different questions
// and because a change in it invalidates a remembered level: a device that has
// gone from discharging to charging has not merely moved a few percent.
type State int

const (
	// Discharging is the ordinary case and says nothing worth drawing.
	Discharging State = iota
	// Charging is plugged in and filling.
	Charging
	// Full is charged, and is separate from Charging because a device says so
	// itself and a panel that showed it as still filling would be wrong for
	// as long as it stayed on the cable.
	Full
)

// String names a state for display and for a test's failure message.
func (s State) String() string {
	switch s {
	case Charging:
		return "charging"
	case Full:
		return "full"
	default:
		return "discharging"
	}
}

// Battery is one device's reading.
type Battery struct {
	// Name is the device's own name, as the device or the kernel gives it,
	// without a leading vendor word where the driver knows the vendor (see
	// Product). It identifies the device across polls, so a level is never
	// carried onto a different device.
	Name string

	// Level is a percentage. HasLevel is false for a device that is present
	// and connected but has not said how full it is, which a headset on a
	// charging cradle does.
	Level    int
	HasLevel bool

	State State

	// Band is how full a device without a fuel gauge says it is, in the four
	// steps such a device knows. HasBand is false for everything with a
	// percentage.
	//
	// **Not a substitute for Level and never converted into one.** A device
	// saying "good" does not mean 75 %; drawing the four steps is how the
	// device's own indicator shows it.
	Band    Band
	HasBand bool

	// Kind is what sort of device this is, where the source could say. A
	// source that cannot tell leaves it KindOther.
	Kind Kind

	// Cells are the batteries inside the device, for one that has more than
	// one: two earbuds and a case. Empty for the ordinary device with a single
	// battery.
	//
	// Level is still the device's number (the lower of the two ears), and
	// these are the detail beneath it. They are kept apart rather than folded
	// into a string because formatting is the consumer's job.
	Cells []Cell
}
