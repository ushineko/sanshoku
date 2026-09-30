package cooling

import (
	"context"
	"time"
)

// Source is a device that can report its cooling status.
type Source interface {
	Status(context.Context) (Status, error)
}

/*
Status is what a liquid cooler reports about itself.

Raw measurements and when they were taken, rather than what any particular
view needs.
*/
type Status struct {
	// Coolant is the liquid temperature in degrees Celsius.
	Coolant float64

	// PumpRPM and FanRPM are speeds; PumpDuty and FanDuty are percentages.
	// Zero means the device did not report one, which is different from a
	// stopped pump and is why HasPump and HasFan exist.
	PumpRPM  int
	PumpDuty int
	FanRPM   int
	FanDuty  int

	HasPump bool
	HasFan  bool

	// Taken is when the reading was taken, which a driver that coalesces
	// reads (docs/design.md, "Freshness") may report as earlier than the call.
	Taken time.Time
}
