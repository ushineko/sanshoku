package battery

/*
Kind is what sort of device a battery belongs to.

hayami orders its cells by it, because a desk has one mouse and several things
that are sometimes there: the mouse is the reading a glance is usually after,
and ordering by name put a headset first.

It is deliberately coarse. It tells a mouse from a keyboard from a headset
from a game controller and nothing else; a taxonomy with a case per protocol
would have to be kept in step with drivers that each name things differently.
The BlueZ icons map onto it as input-mouse, input-keyboard, audio-headset or
audio-headphones, and input-gaming.
*/
type Kind int

const (
	// KindOther is the zero value and means "not one of the below", which
	// covers a device that did not say as well as one that said something
	// with no case here. The two are not distinguished because nothing would
	// be done differently about them.
	KindOther Kind = iota
	// KindMouse is a mouse, and also a trackball or a touchpad: what they
	// have in common is being the pointing device on the desk.
	KindMouse
	// KindKeyboard is a keyboard, and also a numpad.
	KindKeyboard
	// KindHeadset is a headset or a pair of headphones or earbuds.
	KindHeadset
	// KindGamepad is a game controller: a gamepad or a joystick (#47).
	KindGamepad
)

// String names a kind for display and for JSON.
func (k Kind) String() string {
	switch k {
	case KindMouse:
		return "mouse"
	case KindKeyboard:
		return "keyboard"
	case KindHeadset:
		return "headset"
	case KindGamepad:
		return "gamepad"
	default:
		return "other"
	}
}
