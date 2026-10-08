package aula

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

// fromHex is a report written as the sources print them.
func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// The question is the battery command with an empty body, summing to its last
// byte, as PyFlat/Device-Battery-Info PR #6 sends it.
func TestTheRequestIsTheBatteryCommandWithItsChecksum(t *testing.T) {
	assert.Equal(t, fromHex(t, "134a00000000000000000000000000000000005d"), request())
}

/*
The replies, from the captures in PyFlat/Device-Battery-Info PR #6 and from
the receiver on the desk spec 013 was written at.

On battery the level is read. On the cable the receiver pins the level at 100,
and the reading is charging with no level. The receiver's 0x0A status frame
on the same report is not a reply, and neither is a battery reply whose
checksum does not add up.
*/
func TestDecode(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  battery.Battery
		ok    bool
	}{
		{"97 % on battery", "134a0100026101000000000000000000000000c2",
			battery.Battery{Level: 97, HasLevel: true, State: battery.Discharging, Kind: battery.KindKeyboard}, true},
		{"100 % on battery, measured", "134a0100026401000000000000000000000000c5",
			battery.Battery{Level: 100, HasLevel: true, State: battery.Discharging, Kind: battery.KindKeyboard}, true},
		{"the high bit set on the command", "13ca0100026101000000000000000000000000" + "42",
			battery.Battery{Level: 97, HasLevel: true, State: battery.Discharging, Kind: battery.KindKeyboard}, true},
		{"cable in", "134a0100026410000000000000000000000000d4",
			battery.Battery{State: battery.Charging, Kind: battery.KindKeyboard}, true},
		{"a status frame", "130a0100040564010000000000000000000000008c", battery.Battery{}, false},
		{"a corrupt checksum", "134a0100026101000000000000000000000000c3", battery.Battery{}, false},
		{"a level of zero", "134a0100020001000000000000000000000000" + "61", battery.Battery{}, false},
		{"an unknown state", "134a0100026102000000000000000000000000c3", battery.Battery{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode(fromHex(t, c.reply))
			if !c.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
			assert.False(t, c.want.State == battery.Charging && got.HasLevel, "the pinned 100 was read as a level")
		})
	}
}
