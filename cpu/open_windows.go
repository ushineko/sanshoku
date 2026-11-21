//go:build windows

package cpu

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/ushineko/sanshoku/pawnio"
)

var errClosed = errors.New("the reader is closed")

/*
Open loads the module for this machine's processor, as Windows names its
maker and model: AMDFamily17 for AMD, IntelMSR for Intel. The module itself
refuses a processor it does not serve (an AMD older than Zen). PawnIO, the
module and Administrator rights are needed, as for package pawnio.
*/
func Open() (*Reader, error) {
	vendor, name := processor()
	switch vendor {
	case "AuthenticAMD":
		e, err := pawnio.Open("AMDFamily17.bin")
		if err != nil {
			return nil, err //nolint:wrapcheck // pawnio names the module
		}
		return newReader(e, true, name), nil
	case "GenuineIntel":
		e, err := pawnio.Open("IntelMSR.bin")
		if err != nil {
			return nil, err //nolint:wrapcheck // pawnio names the module
		}
		return newReader(e, false, name), nil
	default:
		return nil, fmt.Errorf("a processor from %q: %w", vendor, ErrNoReading)
	}
}

// processor is the first processor's maker and name, where Windows keeps
// them.
func processor() (vendor, name string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer func() { _ = k.Close() }()
	vendor, _, _ = k.GetStringValue("VendorIdentifier")
	name, _, _ = k.GetStringValue("ProcessorNameString")
	return vendor, strings.TrimSpace(name)
}
