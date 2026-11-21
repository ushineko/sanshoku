package pawnio

import (
	"fmt"
)

// The Super I/O chip's configuration ports, and the slot PawnIO's LpcIO
// module numbers each (OpenRGB 1.1's super_io_pawnio.cpp).
var superIOSlots = map[int]uint64{0x2E: 0, 0x4E: 1}

// isaMutex is the name programs take the Super I/O chip in turn by.
const isaMutex = `Global\Access_ISABUS.HTP.Method`

/*
SuperIO is a mainboard's Super I/O chip at one configuration port, through
PawnIO's LpcIO module.

The chip's registers are reached through a logical-device select, so a
sequence of reads and writes is one unit: In holds the ISA bus mutex across
it, so another program (LibreHardwareMonitor reads its sensors the same way)
cannot move the logical device between a select and the writes after it.
*/
type SuperIO struct {
	exec Executor
	port int
	lock func(string) (func(), error)
}

// NewSuperIO selects the chip at port (0x2E or 0x4E) on a loaded LpcIO
// module and enters its configuration mode.
func NewSuperIO(e Executor, port int) (*SuperIO, error) {
	slot, ok := superIOSlots[port]
	if !ok {
		return nil, fmt.Errorf("port %#x is not a Super I/O port LpcIO knows", port)
	}
	if _, err := e.Execute("ioctl_select_slot", []uint64{slot}, 0); err != nil {
		return nil, err //nolint:wrapcheck // errAt names it
	}
	s := &SuperIO{exec: e, port: port, lock: Lock}
	// What OpenRGB does after selecting the slot, before any register.
	for range 2 {
		if _, err := s.inb(0x87); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// In runs fn with the chip to itself: the ISA bus mutex held throughout.
func (s *SuperIO) In(fn func(c Chip) error) error {
	unlock, err := s.lock(isaMutex)
	if err != nil {
		return err
	}
	defer unlock()
	return fn(chip{s})
}

/*
Chip is the Super I/O chip while In holds it: a configuration register read
or written.
*/
type Chip interface {
	Inb(register byte) (byte, error)
	Outb(register, value byte) error
}

type chip struct{ s *SuperIO }

func (c chip) Inb(register byte) (byte, error) { return c.s.inb(register) }
func (c chip) Outb(register, value byte) error { return c.s.outb(register, value) }

func (s *SuperIO) inb(register byte) (byte, error) {
	out, err := s.exec.Execute("ioctl_superio_inb", []uint64{uint64(register)}, 1)
	if err != nil {
		return 0, err //nolint:wrapcheck // errAt names it
	}
	if len(out) != 1 {
		return 0, fmt.Errorf("no value for Super I/O register %#02x", register)
	}
	return byte(out[0]), nil //nolint:gosec // a register byte
}

func (s *SuperIO) outb(register, value byte) error {
	_, err := s.exec.Execute("ioctl_superio_outb", []uint64{uint64(register), uint64(value)}, 0)
	return err //nolint:wrapcheck // errAt names it
}

// Close unloads the module.
func (s *SuperIO) Close() error { return s.exec.Close() }
