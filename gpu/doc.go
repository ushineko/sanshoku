/*
Package gpu reads a graphics card's name, temperature and load from Windows
itself, without a vendor's library and without a subprocess. Spec 018.

The temperature and the name come from the graphics kernel's thunks in gdi32
(D3DKMT), and the load from the "GPU Engine" performance counters: the two
sources Task Manager reads. Any vendor's driver that reports to Windows is
read the same way. Both are plain calls into system libraries through
golang.org/x/sys, so there is no cgo.

Windows only. Elsewhere Read reports errors.ErrUnsupported: Linux reads a
card through hwmon, where the kernel has a driver for it.
*/
package gpu
