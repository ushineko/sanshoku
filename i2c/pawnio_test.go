package i2c

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
smbusModule is a PawnIO SMBus module in memory: it records each transfer's
nine inputs and answers with data it is given, as the module answers with
the data union.
*/
type smbusModule struct {
	calls  [][]uint64
	answer [40]byte
	locks  int
	closed bool
}

func (m *smbusModule) Execute(function string, in []uint64, out int) ([]uint64, error) {
	if function != "ioctl_smbus_xfer" {
		return nil, errors.New("no such function")
	}
	m.calls = append(m.calls, append([]uint64(nil), in...))
	res := make([]uint64, out)
	for i := range res {
		res[i] = binary.LittleEndian.Uint64(m.answer[i*8:])
	}
	return res, nil
}

func (m *smbusModule) Close() error { m.closed = true; return nil }

func newPawnBus(m *smbusModule) *pawnBus {
	return &pawnBus{exec: m, address: 0x5A, name: "PawnIO SMBus piix4 0", lock: func(name string) (func(), error) {
		if name != smbusMutex {
			return nil, errors.New("another mutex")
		}
		m.locks++
		return func() {}, nil
	}}
}

// A byte write is the address, write, the register, byte data, and the byte,
// under the SMBus mutex.
func TestAPawnIOByteWrite(t *testing.T) {
	m := &smbusModule{}
	b := newPawnBus(m)
	require.NoError(t, b.WriteByteData(0xB0, 0x12))
	require.Len(t, m.calls, 1)
	assert.Equal(t, []uint64{0x5A, smbusWrite, 0xB0, smbusByteData, 0x12, 0, 0, 0, 0}, m.calls[0])
	assert.Equal(t, 1, m.locks)
}

// A byte read answers the data union's first byte.
func TestAPawnIOByteRead(t *testing.T) {
	m := &smbusModule{}
	m.answer[0] = 0xBA
	v, err := newPawnBus(m).ReadByteData(0xA0)
	require.NoError(t, err)
	assert.Equal(t, byte(0xBA), v)
	assert.Equal(t, []uint64{0x5A, smbusRead, 0xA0, smbusByteData}, m.calls[0][:4])
}

// Blocks carry their length first, packed little-endian into the five data
// values; an SMBus block is size 5 and an I2C block size 8.
func TestPawnIOBlocks(t *testing.T) {
	m := &smbusModule{}
	b := newPawnBus(m)
	block := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	require.NoError(t, b.WriteSMBusBlock(0x31, block))
	call := m.calls[0]
	assert.Equal(t, uint64(smbusBlockData), call[3])
	var packed [40]byte
	for i := range 5 {
		binary.LittleEndian.PutUint64(packed[i*8:], call[4+i])
	}
	assert.Equal(t, append([]byte{9}, block...), packed[:10])

	require.NoError(t, b.WriteBlockData(0xC1, []byte{4, 0xFF}))
	assert.Equal(t, uint64(smbusI2CBlockData), m.calls[1][3])

	m.answer = [40]byte{3, 0xAA, 0xBB, 0xCC}
	got, err := b.ReadBlockData(0xB1, 6)
	require.NoError(t, err)
	assert.Equal(t, []byte{0xAA, 0xBB, 0xCC}, got, "the device answered three")
	require.ErrorIs(t, b.WriteSMBusBlock(0x31, make([]byte, 33)), ErrBlock)
}
