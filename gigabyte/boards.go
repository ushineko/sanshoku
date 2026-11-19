package gigabyte

/*
zoneDef is one light of a board's layout, as OpenRGB 1.1's board table names
it. A header has a slot (0 or 1: which length class, colour order and effect
bit are its) and a packet byte; a single-colour light has its index, which
names its effect register and its bit in the commit mask.
*/
type zoneDef struct {
	name   string
	header bool
	slot   int
	packet byte
	light  byte
}

// The headers of an IT5701 or IT5702 controller, and their packet bytes.
func header1(name string) zoneDef { return zoneDef{name: name, header: true, slot: 0, packet: 0x58} }
func header2(name string) zoneDef { return zoneDef{name: name, header: true, slot: 1, packet: 0x59} }

// single is a single-colour light at its index.
func single(name string, light byte) zoneDef { return zoneDef{name: name, light: light} }

// boardKey is a board's DMI name and the controller's number on it.
type boardKey struct {
	board      string
	controller byte
}

/*
boards are the layouts of measured boards. The controller does not report
which lights it drives, so a board is here only once someone has looked at it
(spec 039 R1.3). Names and indices are OpenRGB 1.1's for the board.
*/
var boards = map[boardKey][]zoneDef{
	// Spec 039.
	{"Z790 AORUS MASTER X", 0}: {
		header1("ARGB_V2_1/ARGB_V2_3"),
		header2("ARGB_V2_2"),
		single("LED_C", 4),
	},
	{"Z790 AORUS MASTER X", 1}: {
		single("I/O Cover", 0),
	},
}
