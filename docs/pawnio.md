# PawnIO on Windows

How this module reaches the chipset's SMBus and a mainboard's Super I/O chip
on Windows: through PawnIO, a signed kernel driver that runs signed modules
(spec 044). This page is the source for package `pawnio` and the Windows half
of package `i2c`.

## Sources

- **P**: PawnIO's `PawnIOLib.h` (LGPL-2.1), the library's interface.
- **O**: OpenRGB 1.1's PawnIO SMBus and Super I/O code
  (`i2c_smbus/Windows/i2c_smbus_pawnio.cpp`, `super_io/super_io_pawnio.cpp`,
  tag `release_1.1`). GPL, so its code is not translated. See
  [credits.md](credits.md).
- **B**: the bench, recorded in specs 044 to 046.

## What is installed, and where

- PawnIO's installer puts the driver and `PawnIOLib.dll` in its own directory
  under Program Files. This module loads the library from there at run time
  (P). Without it, nothing here is available.
- The modules are signed blobs, one per job: `SmbusI801.bin`,
  `SmbusPIIX4.bin`, `LpcIO.bin` and others, published by PawnIO's author
  (LGPL-2.1). OpenRGB keeps them beside its executable (O). This module reads
  them from `pawnio.ModuleDir`, by default the program's own directory, and
  ships none.
- PawnIO serves a process with Administrator rights only (O).

## The library

`pawnio_open` gives a handle, `pawnio_load` loads a module's blob into it,
`pawnio_execute` runs one of the module's functions by name with up to so many
64-bit values in and out, and `pawnio_close` ends it (P). Each call answers an
HRESULT.

## The SMBus

| Function | In | Out (O) |
|---|---|---|
| `ioctl_identity` | 0 | three values; the third is the controller's PCI vendor, device, subsystem vendor and subsystem device, 16 bits each from the bottom |
| `ioctl_piix4_port_sel` | the port, 0 or 1 | one value |
| `ioctl_set_sleep_mode` | 0 always busy, 1 short busy, 2 always sleep; OpenRGB uses 2, sanshoku 1 (a sleep is Windows' timer tick, about 15 ms) | none |
| `ioctl_smbus_xfer` | the address, read (1) or write (0), the command, the transaction size, and Linux's 34-byte data union in five values | the data union in five values |

The transaction sizes are Linux's: 2 byte data, 5 SMBus block, 8 I2C block.
OpenRGB loads the Intel module once and the AMD one twice, for ports 0 and 1
(O). Every transfer is made holding the global mutex
`Global\Access_SMBUS.HTP.Method`, which LibreHardwareMonitor, HWiNFO and
OpenRGB take for the same bus.

## The processor's temperature (spec 047)

| Module | Function | In | Out (M, L) |
|---|---|---|---|
| `AMDFamily17.bin` | `ioctl_read_smn` | an SMN address | its value |
| `IntelMSR.bin` | `ioctl_read_msr` | an MSR on its read allow-list | its value |

- AMD (families 17h to 1Ah; the module refuses others): SMN `0x59800`, bits
  31:21 at 0.125 °C, less 49 °C where bit 19 is set or bits 17:16 are both
  set (L). The module's comment asks for the `Access_PCI` mutex around an SMN
  read (M), as LibreHardwareMonitor takes it (L). Some processors' Tctl runs
  above the die: 20 °C on the 1600X, 1700X and 1800X, 10 °C on the 2700X,
  27 °C on Threadripper 19xx and 29xx (Linux k10temp, as LibreHardwareMonitor
  uses it).
- Intel: TjMax is MSR `1A2` bits 23:16; the package's distance below it is
  MSR `1B1` bits 22:16, valid when bit 31 is set.
- The modules are in PawnIO.Modules' signed release (0.2.12 on the bench),
  with the SMBus and LpcIO modules.

Sources here: **M**, the modules' source (`AMDFamily17.p`, `IntelMSR.p`,
LGPL-2.1); **L**, LibreHardwareMonitor's `Amd17Cpu.cs` (MPL-2.0). Facts only.

## The Super I/O chip

| Function | In | Out (O) |
|---|---|---|
| `ioctl_select_slot` | 0 for port `0x2E`, 1 for `0x4E` | none |
| `ioctl_superio_inb` | a configuration register | its value |
| `ioctl_superio_outb` | a register and a value | none |

After the slot OpenRGB reads register `87` twice (O). The chip's registers
are reached through a logical-device select (register `07`), so this module
holds the global mutex `Global\Access_ISABUS.HTP.Method` across a whole
sequence: another program that reads the chip's sensors cannot move the
logical device between a select and the writes after it. OpenRGB does not
take that mutex.
