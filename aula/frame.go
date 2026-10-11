package aula

import (
	"slices"

	"github.com/ushineko/sanshoku/lighting"
)

/*
The F75's frames, measured on 2026-10-10 on an F75 with the user watching
(spec 019). The keyboard is lit two ways, by two devices.

Through its 2.4 GHz receiver a frame is one message of command 0x88 on the
receiver's 20-byte report: packets of fourteen body bytes, each carrying how
many packets the message has, which one it is, and a mode in the high nibble
of the length byte. Mode 1 is colours by key, `R G B count slot...` for each
colour; mode 2 is one colour for the board. The command is the one the
vendor's program streams with, read from the captures in OpenRGB issue 5166.

On its cable a frame is one 520-byte feature report, command 0x08: a colour
for each of 126 slots, in slot order. deepan-alve/womier-l65-linux documents
it for another keyboard on the same chip.

A slot is column times six plus row, the F row being row 0, on both.
*/
const (
	// commandFrame is the receiver's frame command.
	commandFrame = 0x88

	// modeKeys and modeBoard are the frame command's two modes.
	modeKeys  = 1
	modeBoard = 2

	// bodyLen is how many body bytes a receiver packet carries.
	bodyLen = 14

	// slotCount is how many slots a wired frame carries. The F75 has a key
	// on 80 of them.
	slotCount = 126

	/*
		messageLimit is how many body bytes one receiver message may carry
		and still be shown whole.

		Measured with a still frame whose last group was the last four keys
		(spec 019 E2): at 224 and 228 bytes all four held; at 232 the last
		two flickered, and at 236 and 240 all four. So the keyboard keeps
		about 230 bytes and what follows is not to be relied on. A second
		message replaces the first, so a frame cannot be split. This is the
		largest message seen whole.
	*/
	messageLimit = 228

	// groupHeader is what a colour costs in a message: R, G, B and a count.
	groupHeader = 4
)

/*
ReceiverColours is how many colours a frame through the receiver can show at
once with every key lit: what is left of one message after a byte for each
key, at four bytes a colour. A frame with more is shown with its nearest
colours merged. On the cable every key has its own colour.
*/
const ReceiverColours = (messageLimit - keyCount) / groupHeader

// colour is one light's colour.
type colour [3]uint8

// black is a light that is off, and what a frame does not name.
var black = colour{}

// frame is what the keyboard is showing: a colour for each slot.
type frame [slotCount]colour

// group is one colour and the slots that show it, in ascending order.
type group struct {
	c     colour
	slots []byte
}

// size is how many body bytes a list of groups takes.
func size(groups []group) int {
	n := 0
	for _, g := range groups {
		n += groupHeader + len(g.slots)
	}
	return n
}

/*
groups is a frame as the receiver takes it: one group for each colour, in the
order of each colour's first slot, so the same frame is always the same
bytes. A black light is in no group, which is how the keyboard shows black.
*/
func groups(f *frame) []group {
	var out []group
	at := map[colour]int{}
	for _, k := range f75Keys {
		c := f[k.ID]
		if c == black {
			continue
		}
		i, ok := at[c]
		if !ok {
			i = len(out)
			at[c] = i
			out = append(out, group{c: c})
		}
		out[i].slots = append(out[i].slots, k.ID)
	}
	return out
}

/*
reduce merges a frame's nearest colours until it fits limit body bytes.

The two groups whose colours are nearest become one, coloured with their mean
weighted by how many keys each lights, until the message fits. Every key that
was lit stays lit. Ties go to the earlier groups, so the same frame reduces
the same way every time. A frame that already fits is returned as it is.
*/
func reduce(in []group, limit int) []group {
	out := slices.Clone(in)
	for size(out) > limit && len(out) > 1 {
		a, b := nearest(out)
		out[a] = merge(out[a], out[b])
		out = slices.Delete(out, b, b+1)
	}
	return out
}

// nearest is the pair of groups whose colours are closest, the earlier pair
// on a tie, with a before b.
func nearest(gs []group) (a, b int) {
	best := -1
	for i := range gs {
		for j := i + 1; j < len(gs); j++ {
			if d := distance(gs[i].c, gs[j].c); best < 0 || d < best {
				best, a, b = d, i, j
			}
		}
	}
	return a, b
}

// distance is the squared distance between two colours.
func distance(x, y colour) int {
	d := 0
	for i := range x {
		v := int(x[i]) - int(y[i])
		d += v * v
	}
	return d
}

// merge is two groups as one: the mean of their colours weighted by their
// keys, and both their slots in order. It stands where a stood, which is the
// earlier first slot.
func merge(a, b group) group {
	na, nb := len(a.slots), len(b.slots)
	var c colour
	for i := range c {
		c[i] = uint8((int(a.c[i])*na + int(b.c[i])*nb + (na+nb)/2) / (na + nb)) //nolint:gosec // a mean of bytes is a byte
	}
	slots := append(slices.Clone(a.slots), b.slots...)
	slices.Sort(slots)
	return group{c: c, slots: slots}
}

/*
receiverFrame is a frame as the packets the receiver is written: one message
of command 0x88, reduced to what a message holds. A frame that is black
everywhere is the one-colour mode with black, since a message naming no key
would be empty.
*/
func receiverFrame(f *frame) [][]byte {
	gs := reduce(groups(f), messageLimit)
	if len(gs) == 0 {
		return [][]byte{packet(commandFrame, 1, 0, modeBoard, black[:])}
	}
	body := make([]byte, 0, size(gs))
	for _, g := range gs {
		body = append(body, g.c[0], g.c[1], g.c[2], byte(len(g.slots))) //nolint:gosec // at most keyCount
		body = append(body, g.slots...)
	}
	total := (len(body) + bodyLen - 1) / bodyLen
	out := make([][]byte, 0, total)
	for i := range total {
		chunk := body[i*bodyLen : min((i+1)*bodyLen, len(body))]
		out = append(out, packet(commandFrame, byte(total), byte(i), modeKeys, chunk)) //nolint:gosec // at most 18 packets
	}
	return out
}

// packet is one 20-byte report: the command, the message's packet count,
// this packet's index, the mode and body length, the body, and the checksum.
func packet(command, total, index, mode byte, body []byte) []byte {
	p := make([]byte, reportLen)
	p[0], p[1], p[2], p[3] = reportID, command, total, index
	p[4] = mode<<4 | byte(len(body)) //nolint:gosec // at most bodyLen
	copy(p[5:], body)
	p[reportLen-1] = checksum(p)
	return p
}

/*
The wired keyboard's feature report: report 6, a command, an index, a zero,
the packet count and index (always one packet), the data's length in two
bytes, and the data, to 520 bytes.
*/
const (
	// wiredReport is the feature report every lighting command is sent on.
	// Report 5 on the same collection is the chip's bootloader channel, and
	// nothing here names it.
	wiredReport = 6

	// wiredLen is the feature report's length, its number included.
	wiredLen = 520

	// wiredHeader is how many bytes come before the data.
	wiredHeader = 8

	// wiredFrameCommand draws a frame from the data: slotCount colours.
	wiredFrameCommand = 0x08

	// wiredModelCommand asks what the keyboard is, with wiredModelIndex.
	wiredModelCommand = 0x82
	wiredModelIndex   = 1
	wiredModelLen     = 6

	// wiredModelAt is where the model is in the answer's data.
	wiredModelAt = 5
)

// wiredRequest is a feature report with its header and no data yet.
func wiredRequest(command, index byte, length int) []byte {
	r := make([]byte, wiredLen)
	r[0], r[1], r[2], r[4] = wiredReport, command, index, 1
	r[6], r[7] = byte(length), byte(length>>8) //nolint:gosec // a length under wiredLen, low byte then high
	return r
}

// wiredFrame is a frame as the feature report the cable is sent: a colour
// for every slot. A slot with no key on it is black.
func wiredFrame(f *frame) []byte {
	r := wiredRequest(wiredFrameCommand, 0, slotCount*3)
	for _, k := range f75Keys {
		copy(r[wiredHeader+3*int(k.ID):], f[k.ID][:])
	}
	return r
}

// keyCount is how many lights the F75 has.
const keyCount = 80

/*
f75Keys are the F75's lights by slot, named by the key on them.

Read from the keyboard: its own key matrix (the wired read 0x83, layer 0)
names the key at each slot, and W, A, S, D, U, Escape and the backtick were
seen to light at the slots it gives them. Slots 75 and 76 hold the two keys
only an ISO board has, and slot 84 the knob's press; this US board has no key
there and they are left out until a bench shows a light on them.
*/
var f75Keys = [keyCount]lighting.Key{
	{ID: 0, Name: "Escape"}, {ID: 1, Name: "`"}, {ID: 2, Name: "Tab"}, {ID: 3, Name: "Caps Lock"},
	{ID: 4, Name: "Left Shift"}, {ID: 5, Name: "Left Control"},
	{ID: 7, Name: "1"}, {ID: 8, Name: "Q"}, {ID: 9, Name: "A"}, {ID: 10, Name: "Z"}, {ID: 11, Name: "Left GUI"},
	{ID: 12, Name: "F1"}, {ID: 13, Name: "2"}, {ID: 14, Name: "W"}, {ID: 15, Name: "S"}, {ID: 16, Name: "X"},
	{ID: 17, Name: "Left Alt"},
	{ID: 18, Name: "F2"}, {ID: 19, Name: "3"}, {ID: 20, Name: "E"}, {ID: 21, Name: "D"}, {ID: 22, Name: "C"},
	{ID: 24, Name: "F3"}, {ID: 25, Name: "4"}, {ID: 26, Name: "R"}, {ID: 27, Name: "F"}, {ID: 28, Name: "V"},
	{ID: 30, Name: "F4"}, {ID: 31, Name: "5"}, {ID: 32, Name: "T"}, {ID: 33, Name: "G"}, {ID: 34, Name: "B"},
	{ID: 35, Name: "Space"},
	{ID: 36, Name: "F5"}, {ID: 37, Name: "6"}, {ID: 38, Name: "Y"}, {ID: 39, Name: "H"}, {ID: 40, Name: "N"},
	{ID: 42, Name: "F6"}, {ID: 43, Name: "7"}, {ID: 44, Name: "U"}, {ID: 45, Name: "J"}, {ID: 46, Name: "M"},
	{ID: 48, Name: "F7"}, {ID: 49, Name: "8"}, {ID: 50, Name: "I"}, {ID: 51, Name: "K"}, {ID: 52, Name: ","},
	{ID: 53, Name: "Fn"},
	{ID: 54, Name: "F8"}, {ID: 55, Name: "9"}, {ID: 56, Name: "O"}, {ID: 57, Name: "L"}, {ID: 58, Name: "."},
	{ID: 59, Name: "Right Control"},
	{ID: 60, Name: "F9"}, {ID: 61, Name: "0"}, {ID: 62, Name: "P"}, {ID: 63, Name: ";"}, {ID: 64, Name: "/"},
	{ID: 66, Name: "F10"}, {ID: 67, Name: "-"}, {ID: 68, Name: "["}, {ID: 69, Name: "'"}, {ID: 70, Name: "Right Shift"},
	{ID: 72, Name: "F11"}, {ID: 73, Name: "="}, {ID: 74, Name: "]"}, {ID: 77, Name: "Left"},
	{ID: 78, Name: "F12"}, {ID: 79, Name: "Backspace"}, {ID: 80, Name: `\`}, {ID: 81, Name: "Enter"},
	{ID: 82, Name: "Up"}, {ID: 83, Name: "Down"},
	{ID: 85, Name: "Delete"}, {ID: 86, Name: "Page Up"}, {ID: 87, Name: "Page Down"}, {ID: 88, Name: "End"},
	{ID: 89, Name: "Right"},
}

// isKey is whether a slot has a light on it.
var isKey = func() (set [slotCount]bool) {
	for _, k := range f75Keys {
		set[k.ID] = true
	}
	return set
}()
