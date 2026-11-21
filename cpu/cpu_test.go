package cpu

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/pawnio"
)

// tctl is a thermal control register reading t degrees, with the -49 range
// flag where asked.
func tctl(t float64, ranged bool) uint32 {
	if ranged {
		t += 49
	}
	raw := uint32(t/0.125) << 21
	if ranged {
		raw |= 0x80000
	}
	return raw
}

func TestTheZenRegister(t *testing.T) {
	assert.InDelta(t, 45.0, amdTctl(tctl(45, false)), 1e-9)
	assert.InDelta(t, 45.0, amdTctl(tctl(45, true)), 1e-9, "the -49 range flag")
	assert.InDelta(t, 45.125, amdTctl(uint32(361)<<21), 1e-9, "eighths of a degree")
	assert.InDelta(t, 51.0-49, amdTctl(uint32(408)<<21|0x30000), 1e-9, "bits 17:16 both set are the range too")
}

// k10temp's offsets: a 2700X's die runs 10 degrees below its Tctl; a 2600X,
// not in the table, is one value labelled for both.
func TestTheOffsets(t *testing.T) {
	v, label := amdReading(tctl(60, false), "AMD Ryzen 7 2700X Eight-Core Processor")
	assert.InDelta(t, 50.0, v, 1e-9)
	assert.Equal(t, LabelTdie, label)

	v, label = amdReading(tctl(60, false), "AMD Ryzen 5 2600X Six-Core Processor")
	assert.InDelta(t, 60.0, v, 1e-9)
	assert.Equal(t, LabelTctlTdie, label)

	v, _ = amdReading(tctl(70, false), "AMD Ryzen Threadripper 2950X 16-Core Processor")
	assert.InDelta(t, 43.0, v, 1e-9)
}

func TestTheIntelPackage(t *testing.T) {
	target := uint64(100) << 16
	v, ok := intelPackage(target, 1<<31|uint64(38)<<16)
	require.True(t, ok)
	assert.InDelta(t, 62.0, v, 1e-9)
	_, ok = intelPackage(target, uint64(38)<<16)
	assert.False(t, ok, "a readout not marked valid")
	_, ok = intelPackage(0, 1<<31)
	assert.False(t, ok, "no TjMax")
}

// module answers a module function from a table.
type module struct {
	answers map[string][]uint64
	calls   []string
}

func (m *module) Execute(function string, in []uint64, _ int) ([]uint64, error) {
	key := function
	if len(in) > 0 {
		key += ":" + string(rune(in[0]))
	}
	m.calls = append(m.calls, function)
	a, ok := m.answers[key]
	if !ok {
		return nil, errors.New("refused")
	}
	return a, nil
}

func (m *module) Close() error { return nil }

// An AMD read is one SMN read of the thermal register, under the PCI mutex.
func TestAnAMDRead(t *testing.T) {
	m := &module{answers: map[string][]uint64{"ioctl_read_smn:" + string(rune(smnThmTconCurTmp)): {uint64(tctl(48.5, false))}}}
	r := newReader(m, true, "AMD Ryzen 5 2600X Six-Core Processor")
	var locked []string
	r.lock = func(name string) (func(), error) { locked = append(locked, name); return func() {}, nil }
	v, label, err := r.Temperature()
	require.NoError(t, err)
	assert.InDelta(t, 48.5, v, 1e-9)
	assert.Equal(t, LabelTctlTdie, label)
	assert.Equal(t, []string{pciMutex}, locked)
	assert.Equal(t, "AMD Ryzen 5 2600X Six-Core Processor", r.Name())
}

// An Intel read is TjMax and the package readout, labelled as LHM labels it.
func TestAnIntelRead(t *testing.T) {
	m := &module{answers: map[string][]uint64{
		"ioctl_read_msr:" + string(rune(msrTemperatureTarget)): {uint64(100) << 16},
		"ioctl_read_msr:" + string(rune(msrPackageThermal)):    {1<<31 | uint64(55)<<16},
	}}
	r := newReader(m, false, "Intel(R) Core(TM) i9-14900K")
	v, label, err := r.Temperature()
	require.NoError(t, err)
	assert.InDelta(t, 45.0, v, 1e-9)
	assert.Equal(t, LabelPackage, label)
}

func TestARefusedReadAndClose(t *testing.T) {
	r := newReader(&module{answers: map[string][]uint64{}}, true, "")
	r.lock = func(string) (func(), error) { return func() {}, nil }
	_, _, err := r.Temperature()
	require.Error(t, err)
	require.NoError(t, r.Close())
	_, _, err = r.Temperature()
	require.Error(t, err)
}

func TestNoPawnIOOffWindows(t *testing.T) {
	_, err := Open()
	require.ErrorIs(t, err, pawnio.ErrNoPawnIO)
}
