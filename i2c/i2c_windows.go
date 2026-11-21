//go:build windows

package i2c

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ushineko/sanshoku/pawnio"
)

/*
smbusModules are PawnIO's SMBus modules this package tries, with the ports a
module serves: the Intel controller has one, AMD's PIIX4-family two (OpenRGB
1.1's i2c_smbus_pawnio.cpp). The Skylake IMC and NCT6793 modules OpenRGB also
loads serve no memory this module drives, and are left out.
*/
var smbusModules = []struct {
	module string
	short  string
	ports  []int // nil: no port to select
}{
	{"SmbusI801.bin", "i801", nil},
	{"SmbusPIIX4.bin", "piix4", []int{0, 1}},
}

// ErrNoI2C is a path that names no PawnIO SMBus.
var ErrNoI2C = errors.New("not a PawnIO SMBus")

/*
Adapters is each SMBus PawnIO can reach on this machine, named as OpenRGB
names it ("PawnIO SMBus piix4 0"), with its controller's PCI IDs from the
module. A module the machine has no controller for, or PawnIO missing or
refusing a process without Administrator rights, is no adapter. Each module
is loaded to ask, and unloaded.
*/
func Adapters() ([]Adapter, error) {
	var out []Adapter
	for _, m := range smbusModules {
		ports := m.ports
		if ports == nil {
			ports = []int{-1}
		}
		for _, port := range ports {
			bus, err := openPawn(m.module, m.short, port)
			if err != nil {
				continue
			}
			ids, err := bus.exec.Execute("ioctl_identity", []uint64{0}, 3)
			_ = bus.Close()
			if err != nil || len(ids) < 3 {
				continue
			}
			index := max(port, 0)
			a := Adapter{
				Path:      fmt.Sprintf("pawnio:%s:%d", m.module, port),
				Name:      fmt.Sprintf("PawnIO SMBus %s %d", m.short, index),
				Vendor:    uint16(ids[2]),       //nolint:gosec // packed IDs
				Device:    uint16(ids[2] >> 16), //nolint:gosec // packed IDs
				SubVendor: uint16(ids[2] >> 32), //nolint:gosec // packed IDs
				SubDevice: uint16(ids[2] >> 48), //nolint:gosec // packed IDs
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// Open opens an adapter Adapters listed and addresses it to a device.
func Open(path string, address uint8) (Bus, error) {
	parts := strings.Split(path, ":")
	if len(parts) != 3 || parts[0] != "pawnio" {
		return nil, fmt.Errorf("%s: %w", path, ErrNoI2C)
	}
	port, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, ErrNoI2C)
	}
	short := ""
	for _, m := range smbusModules {
		if m.module == parts[1] {
			short = m.short
		}
	}
	if short == "" {
		return nil, fmt.Errorf("%s: %w", path, ErrNoI2C)
	}
	bus, err := openPawn(parts[1], short, port)
	if err != nil {
		return nil, err
	}
	bus.address = address
	return bus, nil
}

/*
openPawn loads a module and readies it: the PIIX4 port selected, under the
SMBus mutex, as OpenRGB does, and the module's sleep mode set to short busy
(1). OpenRGB sets always sleep (2), where every wait on the controller is a
kernel sleep, which Windows rounds up to its timer tick (about 15 ms): a
Vengeance colour, five transfers, took 157 ms, and a breathing module drew
three frames a second. Short busy spins through the short waits a transfer
makes and still sleeps on the long ones (#120).
*/
// sleepShortBusy is the modules' SleepMode_ShortBusy (sleepmode.inc).
const sleepShortBusy = 1

func openPawn(module, short string, port int) (*pawnBus, error) {
	e, err := pawnio.Open(module)
	if err != nil {
		return nil, err //nolint:wrapcheck // pawnio names the module
	}
	bus := &pawnBus{exec: e, name: fmt.Sprintf("PawnIO SMBus %s %d", short, max(port, 0)), lock: pawnio.Lock}
	if port >= 0 {
		unlock, err := pawnio.Lock(smbusMutex)
		if err != nil {
			_ = e.Close()
			return nil, err //nolint:wrapcheck // pawnio names the mutex
		}
		_, err = e.Execute("ioctl_piix4_port_sel", []uint64{uint64(port)}, 1) //nolint:gosec // 0 or 1
		unlock()
		if err != nil {
			_ = e.Close()
			return nil, err //nolint:wrapcheck // pawnio names the function
		}
	}
	if _, err := e.Execute("ioctl_set_sleep_mode", []uint64{sleepShortBusy}, 0); err != nil {
		_ = e.Close()
		return nil, err //nolint:wrapcheck // pawnio names the function
	}
	return bus, nil
}
