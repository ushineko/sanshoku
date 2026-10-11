/*
Package lhm reads the processor's temperature from LibreHardwareMonitor's
web server, the route a Windows program has to it without a kernel driver of
its own. Spec 018.

Windows keeps a processor's temperature behind a kernel driver.
LibreHardwareMonitor loads one, through PawnIO, and publishes what it reads
as data.json. This package asks for that tree over loopback and finds the
processor in it. It reads another program's output: it is not a device, and
it makes one kind of request, a GET of the tree.

The parsing builds everywhere. ProbeHost answers on Windows and reports
nothing elsewhere, where LibreHardwareMonitor does not run and hwmon reads
the processor.
*/
package lhm
