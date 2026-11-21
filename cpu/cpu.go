/*
Package cpu reads the processor's temperature on Windows directly, through
PawnIO, the way LibreHardwareMonitor reads it (spec 047): so that a program
that wants it no longer needs LibreHardwareMonitor running.

An AMD Zen processor's control temperature (Tctl) is read from its SMN
register through PawnIO's AMDFamily17 module, with Linux k10temp's per-model
offsets for the die temperature (Tdie). An Intel processor's package
temperature is read from its MSRs through PawnIO's IntelMSR module. Each is
labelled as LibreHardwareMonitor labels it, so a consumer that picked a label
from its tree picks the same one here.

On Linux the kernel's hwmon drivers read the processor (package hwmon), and
Open reports pawnio.ErrNoPawnIO.
*/
package cpu

import (
	"errors"
	"strings"
)

// The labels a reading carries, LibreHardwareMonitor's.
const (
	LabelTctlTdie = "Core (Tctl/Tdie)" // an AMD processor with no offset: the two are one
	LabelTdie     = "Core (Tdie)"      // an AMD processor's die, Tctl less its offset
	LabelPackage  = "CPU Package"      // an Intel processor's package
)

// ErrNoReading is a register that held no temperature this read.
var ErrNoReading = errors.New("the processor gave no temperature")

// smnThmTconCurTmp is the SMN address of the Zen thermal control register
// (LibreHardwareMonitor's F17H_M01H_THM_TCON_CUR_TMP, Linux k10temp).
const smnThmTconCurTmp = 0x00059800

/*
amdTctl decodes the thermal control register: bits 31:21 in steps of 0.125
°C, and 49 °C less where the register says its range is -49 to 206 °C (bit
19, or bits 17:16 both set).
*/
func amdTctl(raw uint32) float64 {
	t := float64(raw>>21) * 0.125
	if raw&0x80000 != 0 || raw&0x30000 == 0x30000 {
		t -= 49
	}
	return t
}

/*
amdOffsets are the processors whose Tctl runs above the die, by a part of
their name (Linux k10temp's tctl_offset_table, which LibreHardwareMonitor
also uses). Any other processor's Tctl is its Tdie.
*/
var amdOffsets = []struct {
	name   string
	offset float64
}{
	{"1600X", 20}, {"1700X", 20}, {"1800X", 20},
	{"2700X", 10},
	{"Threadripper 19", 27}, {"Threadripper 29", 27},
}

// amdReading is an AMD processor's temperature and label: Tdie where the
// processor has an offset, the one value otherwise.
func amdReading(raw uint32, processorName string) (float64, string) {
	t := amdTctl(raw)
	for _, o := range amdOffsets {
		if strings.Contains(processorName, o.name) {
			return t - o.offset, LabelTdie
		}
	}
	return t, LabelTctlTdie
}

// The Intel MSRs read (IntelMSR's read allow-list has both).
const (
	msrTemperatureTarget = 0x1A2 // TjMax in bits 23:16
	msrPackageThermal    = 0x1B1 // the distance below TjMax in bits 22:16, valid when bit 31 is set
)

/*
intelPackage is the package temperature: TjMax less the package's digital
readout. A readout not marked valid, or a TjMax of zero, is no reading.
*/
func intelPackage(target, status uint64) (float64, bool) {
	tjMax := float64(target >> 16 & 0xFF)
	if tjMax == 0 || status&(1<<31) == 0 {
		return 0, false
	}
	return tjMax - float64(status>>16&0x7F), true
}
