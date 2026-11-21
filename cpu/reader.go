package cpu

import (
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku/pawnio"
)

// pciMutex is the name programs take the PCI configuration space in turn by;
// an SMN read goes through it.
const pciMutex = `Global\Access_PCI`

/*
Reader reads one processor's temperature through a loaded PawnIO module. It
keeps the module loaded between reads, as a consumer polls. Close unloads it.
*/
type Reader struct {
	mu   sync.Mutex
	exec pawnio.Executor
	amd  bool
	name string
	lock func(string) (func(), error)
}

// newReader is a Reader over a loaded module: AMDFamily17 when amd, else
// IntelMSR.
func newReader(e pawnio.Executor, amd bool, name string) *Reader {
	return &Reader{exec: e, amd: amd, name: name, lock: pawnio.Lock}
}

// Name is the processor's own name, as Windows keeps it.
func (r *Reader) Name() string { return r.name }

/*
Temperature is the processor's temperature in degrees and its label. An AMD
read holds the PCI mutex, as LibreHardwareMonitor does around an SMN read.
*/
func (r *Reader) Temperature() (float64, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.exec == nil {
		return 0, "", fmt.Errorf("the processor's temperature: %w", errClosed)
	}
	if r.amd {
		unlock, err := r.lock(pciMutex)
		if err != nil {
			return 0, "", err //nolint:wrapcheck // pawnio names the mutex
		}
		out, err := r.exec.Execute("ioctl_read_smn", []uint64{smnThmTconCurTmp}, 1)
		unlock()
		if err != nil || len(out) != 1 {
			return 0, "", fmt.Errorf("the processor's temperature: %w", firstErr(err, ErrNoReading))
		}
		t, label := amdReading(uint32(out[0]), r.name) //nolint:gosec // a 32-bit register
		return t, label, nil
	}
	target, err := r.exec.Execute("ioctl_read_msr", []uint64{msrTemperatureTarget}, 1)
	if err != nil || len(target) != 1 {
		return 0, "", fmt.Errorf("the processor's TjMax: %w", firstErr(err, ErrNoReading))
	}
	status, err := r.exec.Execute("ioctl_read_msr", []uint64{msrPackageThermal}, 1)
	if err != nil || len(status) != 1 {
		return 0, "", fmt.Errorf("the processor's package temperature: %w", firstErr(err, ErrNoReading))
	}
	t, ok := intelPackage(target[0], status[0])
	if !ok {
		return 0, "", ErrNoReading
	}
	return t, LabelPackage, nil
}

// Close unloads the module.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.exec == nil {
		return nil
	}
	err := r.exec.Close()
	r.exec = nil
	return err //nolint:wrapcheck // pawnio names it
}

func firstErr(err, otherwise error) error {
	if err != nil {
		return err
	}
	return otherwise
}
