package i2c

import (
	"encoding/binary"
	"fmt"

	"github.com/ushineko/sanshoku/pawnio"
)

/*
pawnBus is a device at one address on an SMBus reached through PawnIO, which
is how Windows reaches it (spec 044). PawnIO's SMBus modules take Linux's
i2c_smbus_xfer: the address, read or write, the command, the transaction
size, and the 34-byte data union, as nine 64-bit values; they answer the data
union as five (OpenRGB 1.1's i2c_smbus_pawnio.cpp).

Every transfer takes the global SMBus mutex, so that other programs on the
bus (LibreHardwareMonitor, HWiNFO, OpenRGB) take turns with it.
*/
type pawnBus struct {
	exec    pawnio.Executor
	address uint8
	name    string
	lock    func(string) (func(), error)
}

// smbusMutex is the name programs take the SMBus in turn by.
const smbusMutex = `Global\Access_SMBUS.HTP.Method`

func (b *pawnBus) xfer(readWrite uint8, register byte, size uint32, data *[34]byte) error {
	unlock, err := b.lock(smbusMutex)
	if err != nil {
		return err //nolint:wrapcheck // pawnio names the mutex
	}
	defer unlock()
	in := make([]uint64, 9)
	in[0], in[1], in[2], in[3] = uint64(b.address), uint64(readWrite), uint64(register), uint64(size)
	var packed [40]byte
	copy(packed[:], data[:])
	for i := range 5 {
		in[4+i] = binary.LittleEndian.Uint64(packed[i*8:])
	}
	out, err := b.exec.Execute("ioctl_smbus_xfer", in, 5)
	if err != nil {
		return fmt.Errorf("register 0x%02x at 0x%02x on %s: %w", register, b.address, b.name, err)
	}
	for i, v := range out {
		binary.LittleEndian.PutUint64(packed[i*8:], v)
	}
	copy(data[:], packed[:])
	return nil
}

func (b *pawnBus) WriteByteData(register, value byte) error {
	var data [34]byte
	data[0] = value
	return b.xfer(smbusWrite, register, smbusByteData, &data)
}

func (b *pawnBus) WriteBlockData(register byte, block []byte) error {
	if err := checkBlock(block); err != nil {
		return err
	}
	var data [34]byte
	data[0] = byte(len(block)) //nolint:gosec // at most 32
	copy(data[1:], block)
	return b.xfer(smbusWrite, register, smbusI2CBlockData, &data)
}

func (b *pawnBus) WriteSMBusBlock(register byte, block []byte) error {
	if err := checkBlock(block); err != nil {
		return err
	}
	var data [34]byte
	data[0] = byte(len(block)) //nolint:gosec // at most 32
	copy(data[1:], block)
	return b.xfer(smbusWrite, register, smbusBlockData, &data)
}

func (b *pawnBus) ReadByteData(register byte) (byte, error) {
	var data [34]byte
	if err := b.xfer(smbusRead, register, smbusByteData, &data); err != nil {
		return 0, err
	}
	return data[0], nil
}

func (b *pawnBus) ReadBlockData(register byte, n int) ([]byte, error) {
	if err := checkLen(n); err != nil {
		return nil, err
	}
	var data [34]byte
	data[0] = byte(n) //nolint:gosec // at most 32
	if err := b.xfer(smbusRead, register, smbusI2CBlockData, &data); err != nil {
		return nil, err
	}
	got := min(int(data[0]), n)
	return append([]byte(nil), data[1:1+got]...), nil
}

func (b *pawnBus) Close() error { return b.exec.Close() } //nolint:wrapcheck // pawnio names itself
