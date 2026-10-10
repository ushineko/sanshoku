package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The adapter in these names is the card D3DKMT reported on the desk hayami's
// spec 034 was measured on; the processes are invented.
const card3060 = "0x00000000_0x0000e985"

// An instance name is the adapter, the engine and the engine's type.
func TestAGPUEngineInstanceNamesItsAdapterAndEngine(t *testing.T) {
	in, ok := parseEngineInstance("pid_4120_luid_0x00000000_0x0000E985_phys_0_eng_3_engtype_3D")
	require.True(t, ok)
	assert.Equal(t, engineInstance{LUID: card3060, Engine: "phys_0_eng_3", Type: "3D"}, in)

	_, ok = parseEngineInstance("_Total")
	assert.False(t, ok, "a name that is not an instance is not read as one")
}

// D3DKMT's LUID and the counter's are the same adapter, written the same way.
func TestAnAdaptersLUIDIsWrittenAsTheCounterWritesIt(t *testing.T) {
	assert.Equal(t, card3060, luidString(0, 0xE985))
	assert.Equal(t, "0xffffffff_0x00000001", luidString(-1, 1))
}

// The load is the busiest engine, each engine summed over the processes using
// it, as Task Manager gives it: not the sum of every engine, and not the mean.
// Another adapter's engines are not this one's.
func TestTheCardsLoadIsItsBusiestEngine(t *testing.T) {
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          20,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          15,
		"pid_12_luid_0x00000000_0x0000E985_phys_0_eng_4_engtype_VideoDecode": 30,
		"pid_12_luid_0x00000000_0x0000E985_phys_0_eng_1_engtype_Copy":        5,
		"pid_13_luid_0x00000000_0x0000FB45_phys_0_eng_0_engtype_3D":          90,
	}

	load, ok := busiestEngine(values, card3060)

	require.True(t, ok)
	assert.InDelta(t, 35, load, 0.001)
}

// An engine summed past 100 on the counters' rounding is held to 100, and an
// adapter with no instance has no load rather than a load of zero.
func TestTheCardsLoadIsHeldTo100AndAbsentWithoutInstances(t *testing.T) {
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 70,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 40,
	}
	load, ok := busiestEngine(values, card3060)
	require.True(t, ok)
	assert.InDelta(t, 100, load, 0.001)

	_, ok = busiestEngine(values, "0x00000000_0x00000001")
	assert.False(t, ok)
}
