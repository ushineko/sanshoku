//go:build !windows

package lhm

// ProbeHost finds nothing off Windows: LibreHardwareMonitor and PawnIO are
// Windows programs, and Linux reads the processor through hwmon.
func ProbeHost() Host { return Host{} }
