package battery

// CellKind names one battery inside a device.
type CellKind int

const (
	// Headset is a device that is one piece and has one battery.
	Headset CellKind = iota
	// Left is the left earbud.
	Left
	// Right is the right earbud.
	Right
	// Case is the charging case, which is often not there at all.
	Case
)

// String names a cell for display and for a test's failure message.
func (c CellKind) String() string {
	switch c {
	case Left:
		return "L"
	case Right:
		return "R"
	case Case:
		return "case"
	default:
		return "headset"
	}
}

// Cell is one cell's level. A cell that is not there (a case left on the desk)
// is left out by the driver, never reported at zero.
type Cell struct {
	Cell     CellKind
	Level    int
	Charging bool
}
