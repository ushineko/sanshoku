# 042 — Corsair RGB memory over the chipset's SMBus

**Issue**: #108

## Status: COMPLETE

## Executive Summary

A new `corsairdram` driver lights Corsair RGB memory over the chipset's
SMBus: each module a canvas by direct frames. Modules are found by reads
only, once per process, and written only through the four registers that read
device information and take frames. Four Dominator Platinum RGB DDR5 modules
are tested; eight other families are expected. Review the probing and the
write allow-list in `corsairdram/driver.go` first.

## Context

hotaru 2.0 dropped OpenRGB, and the Linux test machine's four Corsair
Dominator Platinum RGB DDR5 modules went dark (hotaru #231). They sit at
`0x18` to `0x1B` on the Intel Raptor Lake SMBus (`8086:7a23`, `i2c-i801`),
whose `0x50` to `0x53` the kernel's `spd5118` driver owns. OpenRGB lit 12 LEDs
on each. The maintainer asked for the driver to be built from OpenRGB's
protocol and checked on the machine in one run, and for the other products
OpenRGB lists to come in as Expected (2026-11-20).

The protocol is in `docs/corsair-dram.md`. It builds on spec 041's additions
to package `i2c`.

## Requirements

### R1. The driver

- R1.1 A `corsairdram` package and `corsairdram.Driver`, named
  "corsairdram", in `all.Drivers`.
- R1.2 Find looks only on the SMBus controllers OpenRGB 1.1 lists for memory,
  by PCI ID, at `0x58` to `0x5F` and `0x18` to `0x1F`, 10 ms apart. At each,
  it opens the address alone (the kernel refuses one its own driver owns) and
  reads `43` and `44`; a module answers `1A`–`1C` and `01`/`03`/`04`. Only
  reads. Each adapter is probed once per process: modules do not come and go
  while a machine runs, and a consumer that looks every few seconds should
  not read the memory bus each time.
- R1.3 Open reads the device-information block (writing `61` and `21`, then
  32 reads of `40`) and its CRC (`42`). A bad CRC, a product ID not in the
  table, or a protocol before 4 (no direct frames) is refused.
- R1.4 Names: the product as OpenRGB names it and the address, `Corsair
  Dominator Platinum RGB DDR5 (0x18)`, which hotaru 0.1.29 showed.
- R1.5 **Writes only to `61`, `21`, `31` and `32`.** The write path refuses
  every other register, among them the bootloader switch (`23`) and the
  configuration write (`82`).
- R1.6 The products: the 26 product IDs of OpenRGB 1.1's nine families, with
  their LED counts and order.

### R2. The canvas

- R2.1 One zone, `Corsair DRAM`, a line of the module's LEDs, keys `Corsair
  DRAM LED 0` upward, as OpenRGB names them. Not resizable.
- R2.2 A frame that changed is sent whole: the count, the LEDs in R, G, B
  (last first where the product says), and a CRC-8, 32 bytes to `31` and the
  rest to `32`.
- R2.3 `Floor` is 50 ms, and a module holds its frame: not `Fading` (the
  bench run).
- R2.4 `Release` forgets the frame and sends nothing. `Close` sends nothing.

### R3. Documentation

- R3.1 `docs/corsair-dram.md`, credits, udev rules for the chipset's SMBus
  adapters, README row and changelog, `devices.md` through `Support()`.

## Acceptance Criteria

- [x] The checks as spec 041's.
- [x] Tests on fake modules: find probes only the memory bus, only the
      addresses, only by reads, and once; an address the kernel owns and
      something else answering are not modules; open names the module from
      its product; a bad CRC, an unknown product and an old protocol are
      refused; a frame's layout, order, CRC and blocks; an unchanged frame
      sends nothing; only the allowed registers are written; CRC-8 against
      its check value.
- [x] Live on the test machine: `sanshoku-bench light --yes --driver
      corsairdram` finds the four modules and shows steady, breathe and wave
      on them; the floor and the hold are measured.
- [x] Docs per R3.

## Out of scope

- The modules' own effects, and protocol 1 to 3 modules, which take only
  effects.
- OpenRGB's fallback for adapters without SMBus block writes.
- Windows (#108's Vengeance, which needs #110).

## Risks & Assumptions

- **The memory bus.** A wrong write there is what makes memory lighting
  dangerous. The driver reads to identify, never opens an address the kernel
  owns, and writes only four registers, on a module that answered both
  identification registers and a device-information block with a good CRC.
- **Reads at Find.** No other driver reads to find. On a machine without
  Corsair memory, Find reads two registers at up to 16 addresses, once.
- **The udev rule** gives the logged-in user the chipset's SMBus, as
  OpenRGB's rules do. Addresses a kernel driver owns stay refused.
- **Rollback:** revert; nothing else uses the package.

## Verification

### Bench run (2026-11-20, the Linux test machine, hotaru and OpenRGB stopped, the maintainer watching)

- `sanshoku-bench light --yes --driver corsairdram --hold 5s` found four
  modules on `/dev/i2c-21`, at `0x18`, `0x19`, `0x1A` and `0x1B`, and opened
  each: device information with a good CRC, a known product, protocol 4 or
  later, 12 keys. The SPD hubs at `0x50` to `0x53` are the kernel's
  (`spd5118`), and probing does not touch them.
- Steady, breathe and wave at 50 ms on each, one module after another, 100
  frames each, none timed out; a frame wrote in 6.1 to 7.1 ms at the median,
  8.0 ms at most. The maintainer: all four showed all three.
- The hold: nothing sent for three minutes after the last module: "Last
  frame held". Not `Fading`.
- The bench prints a candidate's name, which before open is `Corsair memory
  (0x18)`; the opened name (`Corsair Dominator Platinum RGB DDR5 (0x18)`) is
  covered by the tests.
- hotaru and OpenRGB were restarted after; hotaru 0.1.29 found the modules
  again.
