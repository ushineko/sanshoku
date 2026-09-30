package apple

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/battery"
)

// The packet a real pair of AirPods Pro sent hayami, with the levels changed so
// the three cells are told apart and nothing here is a recording of anybody's
// hardware: right 90, left 80, case not present.
//
//	04 00 04 00 04 00  prefix
//	03                 three cells
//	02 01 5a 02 01     right, 90, draining
//	04 01 50 02 01     left, 80, draining
//	08 01 00 04 01     case, 0, not present
const airpodsPacket = "040004000400030201" + "5a0201" + "0401500201" + "080100" + "0401"

func packet(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

/*
DecodeBattery, from hayami's aap_test.go.

A battery packet decodes to the cells it carries, in the order sent. A cell
that is not there — the case on a desk reports level 0 with "not on body" — is
not a cell at zero. Charging is carried per cell. A cell number this build does
not know is skipped rather than guessed at. A packet that is not a battery
packet, is cut short, or is empty is an error rather than a partial reading.
*/
func TestDecodeBattery(t *testing.T) {
	for _, c := range []struct {
		name  string
		pkt   string
		cells []battery.Cell
		fails bool
	}{
		{"two ears, the case not present", airpodsPacket,
			[]battery.Cell{{Cell: battery.Right, Level: 90}, {Cell: battery.Left, Level: 80}}, false},
		{"a cell on the cable says so", "040004000400" + "01" + "0401320101",
			[]battery.Cell{{Cell: battery.Left, Level: 50, Charging: true}}, false},
		{"a one-piece device is one headset cell", "040004000400" + "01" + "0101400201",
			[]battery.Cell{{Cell: battery.Headset, Level: 64}}, false},
		{"an unknown cell is skipped", "040004000400" + "02" + "0401320201" + "7f01630201",
			[]battery.Cell{{Cell: battery.Left, Level: 50}}, false},
		{"a level above 100 is skipped", "040004000400" + "02" + "0401320201" + "0201650201",
			[]battery.Cell{{Cell: battery.Left, Level: 50}}, false},
		{"not a battery packet", "040004000900" + "01" + "0401320101", nil, true},
		{"promises three cells, carries one", "040004000400" + "03" + "0401320101", nil, true},
		{"the prefix and no count", "040004000400", nil, true},
		{"an empty packet", "", nil, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			cells, err := DecodeBattery(packet(t, c.pkt))
			if c.fails {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.cells, cells)
		})
	}
}

/*
The reading's number is the lower ear, and the case does not set it.

The lower ear is the one that will stop working first. A case at 5 % while the
ears are full is not a warning about anything the wearer is doing. The cells
come out in a fixed order, left, right, case, not the order sent: the AirPods
hayami measured report right before left. Only the case answering is still a
reading; nothing answering is not one.
*/
func TestTheReadingIsTheLowerEarWithTheCellsInOrder(t *testing.T) {
	b, err := batteryOf("AirPods", []battery.Cell{
		{Cell: battery.Case, Level: 5},
		{Cell: battery.Right, Level: 90},
		{Cell: battery.Left, Level: 55, Charging: true},
	})
	require.NoError(t, err)
	assert.Equal(t, 55, b.Level)
	assert.True(t, b.HasLevel)
	assert.Equal(t, battery.Charging, b.State)
	var order []string
	for _, c := range b.Cells {
		order = append(order, c.Cell.String())
	}
	assert.Equal(t, []string{"L", "R", "case"}, order)

	b, err = batteryOf("AirPods", []battery.Cell{{Cell: battery.Case, Level: 40, Charging: true}})
	require.NoError(t, err)
	assert.Equal(t, 40, b.Level, "only the case answered, and it is the reading")
	assert.Equal(t, battery.Discharging, b.State, "the case charging is not the ears charging")

	_, err = batteryOf("AirPods", nil)
	require.ErrorIs(t, err, ErrNoBatteryPacket)
}
