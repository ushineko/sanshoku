package pawnio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a module that records each call and answers zero.
type recorder struct {
	calls []string
	ins   [][]uint64
}

func (r *recorder) Execute(function string, in []uint64, out int) ([]uint64, error) {
	r.calls = append(r.calls, function)
	r.ins = append(r.ins, in)
	return make([]uint64, out), nil
}

func (r *recorder) Close() error { return nil }

// The chip at 0x4E is LpcIO's slot 1; another port is refused.
func TestSuperIOSelectsItsSlot(t *testing.T) {
	r := &recorder{}
	_, err := NewSuperIO(r, 0x4E)
	require.NoError(t, err)
	assert.Equal(t, "ioctl_select_slot", r.calls[0])
	assert.Equal(t, []uint64{1}, r.ins[0])

	_, err = NewSuperIO(&recorder{}, 0x3E)
	require.Error(t, err)
}

// In holds the ISA bus mutex across the whole sequence, and releases it.
func TestInHoldsTheISABus(t *testing.T) {
	r := &recorder{}
	s, err := NewSuperIO(r, 0x2E)
	require.NoError(t, err)
	var held []string
	released := 0
	s.lock = func(name string) (func(), error) {
		held = append(held, name)
		return func() { released++ }, nil
	}
	require.NoError(t, s.In(func(c Chip) error {
		assert.Equal(t, 0, released, "released before the sequence ended")
		require.NoError(t, c.Outb(0x07, 0x12))
		_, err := c.Inb(0xE0)
		return err
	}))
	assert.Equal(t, []string{isaMutex}, held)
	assert.Equal(t, 1, released)
	assert.Equal(t, []uint64{0x07, 0x12}, r.ins[len(r.ins)-2])
}

// Off Windows there is no PawnIO.
func TestNoPawnIOHere(t *testing.T) {
	_, err := Open("LpcIO.bin")
	require.ErrorIs(t, err, ErrNoPawnIO)
}
