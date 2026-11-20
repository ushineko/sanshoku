//go:build linux

package i2c

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// sysClass is where Linux lists the i2c-dev adapters. A variable so that a
// test can give its own.
var sysClass = "/sys/class/i2c-dev"

/*
Adapters lists the i2c-dev adapters, each with its name and, where it belongs
to a PCI device, that device's IDs. Nothing is opened.
*/
func Adapters() ([]Adapter, error) {
	entries, err := os.ReadDir(sysClass)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing I2C adapters: %w", err)
	}
	var out []Adapter
	for _, e := range entries {
		dir := filepath.Join(sysClass, e.Name())
		a := Adapter{Path: "/dev/" + e.Name(), Name: readTrim(filepath.Join(dir, "name"))}
		if adapter, err := filepath.EvalSymlinks(filepath.Join(dir, "device")); err == nil {
			pci := filepath.Dir(adapter)
			a.Vendor = readHex(filepath.Join(pci, "vendor"))
			a.Device = readHex(filepath.Join(pci, "device"))
			a.SubVendor = readHex(filepath.Join(pci, "subsystem_vendor"))
			a.SubDevice = readHex(filepath.Join(pci, "subsystem_device"))
		}
		out = append(out, a)
	}
	return out, nil
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readHex(path string) uint16 {
	v, err := strconv.ParseUint(strings.TrimPrefix(readTrim(path), "0x"), 16, 16)
	if err != nil {
		return 0
	}
	return uint16(v)
}

const (
	ioctlSlave = 0x0703
	ioctlSMBus = 0x0720
)

// smbusIoctl is struct i2c_smbus_ioctl_data.
type smbusIoctl struct {
	readWrite uint8
	command   uint8
	_         [2]byte
	size      uint32
	data      uintptr
}

// handle is an open adapter, addressed to one device.
type handle struct {
	f    *os.File
	path string
}

// Open opens an adapter and addresses it to a device.
func Open(path string, address uint8) (Bus, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := ioctl(f, ioctlSlave, uintptr(address)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("addressing 0x%02x on %s: %w", address, path, err)
	}
	return &handle{f: f, path: path}, nil
}

func (h *handle) WriteByteData(register, value byte) error {
	var data [34]byte
	data[0] = value
	return h.smbus(register, smbusByteData, &data)
}

func (h *handle) WriteBlockData(register byte, block []byte) error {
	if err := checkBlock(block); err != nil {
		return err
	}
	var data [34]byte
	data[0] = byte(len(block)) //nolint:gosec // at most 32, checked above
	copy(data[1:], block)
	return h.smbus(register, smbusI2CBlockData, &data)
}

func (h *handle) WriteSMBusBlock(register byte, block []byte) error {
	if err := checkBlock(block); err != nil {
		return err
	}
	var data [34]byte
	data[0] = byte(len(block)) //nolint:gosec // at most 32, checked above
	copy(data[1:], block)
	return h.smbus(register, smbusBlockData, &data)
}

func (h *handle) ReadByteData(register byte) (byte, error) {
	var data [34]byte
	if err := h.call(smbusRead, register, smbusByteData, &data); err != nil {
		return 0, fmt.Errorf("reading register 0x%02x on %s: %w", register, h.path, err)
	}
	return data[0], nil
}

func (h *handle) ReadBlockData(register byte, n int) ([]byte, error) {
	if err := checkLen(n); err != nil {
		return nil, err
	}
	var data [34]byte
	data[0] = byte(n) //nolint:gosec // at most 32, checked above
	if err := h.call(smbusRead, register, smbusI2CBlockData, &data); err != nil {
		return nil, fmt.Errorf("reading register 0x%02x on %s: %w", register, h.path, err)
	}
	got := min(int(data[0]), n)
	return append([]byte(nil), data[1:1+got]...), nil
}

func (h *handle) smbus(register byte, size uint32, data *[34]byte) error {
	if err := h.call(smbusWrite, register, size, data); err != nil {
		return fmt.Errorf("writing register 0x%02x on %s: %w", register, h.path, err)
	}
	return nil
}

// call is one SMBus transaction. The ioctl takes its argument and its data by
// address.
func (h *handle) call(readWrite uint8, register byte, size uint32, data *[34]byte) error {
	arg := smbusIoctl{readWrite: readWrite, command: register, size: size, data: uintptr(unsafe.Pointer(data))} //nolint:gosec // see above
	err := ioctl(h.f, ioctlSMBus, uintptr(unsafe.Pointer(&arg)))                                                //nolint:gosec // see above
	runtime.KeepAlive(data)
	return err
}

func (h *handle) Close() error {
	return h.f.Close() //nolint:wrapcheck // the file names itself
}

func ioctl(f *os.File, req, arg uintptr) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), req, arg)
	if errno != 0 {
		return errno
	}
	return nil
}
