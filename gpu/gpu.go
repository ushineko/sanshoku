package gpu

import (
	"time"

	"github.com/ushineko/sanshoku"
)

/*
Card is one reading of the graphics card. Either figure can be missing: a
driver that reports no temperature, or a machine without the counter set,
leaves the other one standing.
*/
type Card struct {
	// Name is the card's model as its driver registered it, "NVIDIA GeForce
	// RTX 3060 Ti", and empty where the driver gave none.
	Name string

	// Temperature is in degrees Celsius, to a tenth.
	Temperature    float64
	HasTemperature bool

	// Load is the busiest engine's utilisation as a percentage, a mean since
	// the previous Read. See Reader.
	Load    float64
	HasLoad bool
}

/*
Warmup is how far apart two collections of the load counter are at the
least, and so how long a Reader's first Read takes.

The engines' running time is counted coarsely. Over a few milliseconds a busy
card reads 0 %: two collections back to back said nothing true, on the RTX
3060 Ti hayami's spec 034 was measured on. 200 ms is the interval hayami
already used for the processor's first sample, and at it the figure agreed
with Task Manager's.
*/
const Warmup = 200 * time.Millisecond

// ErrNoCard is a machine with no hardware adapter: a virtual machine with
// the Basic Render Driver and nothing else. It wraps sanshoku.ErrAbsent, and
// a program shows the rest of what it reads.
var ErrNoCard error = absent("no graphics card on this machine")

// absent is an error that is sanshoku.ErrAbsent underneath without saying
// "absent" in its message.
type absent string

func (a absent) Error() string { return string(a) }
func (absent) Unwrap() error   { return sanshoku.ErrAbsent }
