# 029 — The MSI RTX 4090's light over I2C

**Issue**: #78

## Status: COMPLETE

## Context

The MSI GeForce RTX 4090 Suprim Liquid X's light is lit through OpenRGB on
the desk this module is written on. Retiring OpenRGB (hotaru #212) needs a
native driver, and the card is the one device on the desk that is not HID: its
lighting controller is at `0x68` on the card's own I2C adapter. Two things
are new:

- **An I2C transport.** The kernel's `i2c-dev` adapters, found with the PCI
  device they belong to, and SMBus byte-data and I2C block writes.
- **The card's own effects.** It has one light and takes about four colours a
  second at most (hotaru's spike, #205), so a moving effect is the card's. The
  maintainer's decision on #212: every device is drawn as a canvas, and the
  graphics card alone also carries its firmware modes.

The maintainer's scenes use the card two ways: a colour on `GPU[0]`, and
Breathing as an effect. Names follow OpenRGB's (the card "MSI GeForce RTX 4090
Suprim Liquid X", zone "GPU", light "Color", effects by OpenRGB's names), so
they find the same things. The protocol is `docs/msi-gpu.md`.

## Requirements

### R1. The transport

- R1.1 An `i2c` package: `Adapters` lists `/sys/class/i2c-dev`, each with its
  name and the IDs of the PCI device it belongs to; `Open` addresses a device
  on an adapter; `WriteByteData` and `WriteBlockData` (at most 32 bytes) are
  the SMBus ioctl. Linux only; elsewhere no adapters.
- R1.2 `sanshoku.BusI2C`.

### R2. The driver

- R2.1 An `msi` package and `msi.Driver`, named "msi", in `all.Drivers`. A
  candidate is an adapter named "NVIDIA i2c adapter 1 …" on a card the
  allow-list names by vendor, device, subsystem vendor and subsystem device;
  the identity is the subsystem's (`1462:5104`). Nothing is read from the
  controller.
- R2.2 A canvas of one light ("Color"), one zone ("GPU"). A colour is the
  static mode: `2E` 00, `22` 1C, `30`–`32`, `36` 100, `22` 13, with 20 ms after
  each write. The same colour again sends nothing. `Floor` is 250 ms. Not
  `Fading`.
- R2.3 `lighting.Onboard` and `lighting.Catalogue`: the 17 modes OpenRGB
  offers this card, written as OpenRGB writes them, at full brightness and
  medium speed, starting with OpenRGB's colours (red, green, blue). `Effect` is
  what this handle set, else `lighting.ErrUnknownEffect`. `SetColour` replaces
  the first colour; an effect with none is `lighting.ErrNoColour`. A colour
  after an effect is the static mode again.
- R2.4 `Release` and `Close` send nothing; the card keeps what it shows. Save
  (`3F`) is never written.

### R3. Documentation and packaging

- R3.1 `docs/msi-gpu.md`; credits; a udev rule giving the seat the NVIDIA
  adapters; README; `devices.md`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make check-api`, `make check-support`,
      `make check-no-binaries`, `CGO_ENABLED=0 make build`, and `go vet` for
      windows, linux and darwin pass.
- [x] Tests: adapters read from a sysfs layout; a block over 32 bytes refused;
      only adapter 1 of an allow-listed card found; the static colour's writes,
      and none for the same colour; Breathing, Flashing, Rainbow Wave, Magic,
      Wave, Color Cycle and Off written as OpenRGB writes them; a colour into
      Breathing's first block; the errors; release and close write nothing.
- [x] Live, with someone watching (OpenRGB stopped): colours through
      `sanshoku-bench light --yes --driver msi` show; Breathing and Rainbow
      Wave through `sanshoku-bench onboard --driver msi --effect … --yes` run;
      Breathing's colour changes.
- [x] Docs per R3.

## Out of scope

- Brightness and speed other than full and medium, and direction.
- Save to the card (`3F`).
- Other MSI cards and the RTX 50 series' extra modes.
- Windows: no `i2c-dev`.

## Risks & Assumptions

- **Writes are blind.** The controller answers nothing, so a wrong address or
  card is not detected; the allow-list by four PCI IDs and the adapter's name
  is the guard. Only this card on this desk is tested.
- **The adapter's number** (`i2c-3`) can change between boots; it is found by
  name and PCI parent each time.
- **Rollback:** revert; nothing else uses the packages.

## Verification

### Tests

`i2c/i2c_linux_test.go`: an adapter's name and its PCI device's four IDs read
from a sysfs layout; a block of 33 bytes refused. `msi/msi_test.go` on a bus
that records every write: only adapter 1 of the allow-listed card found, named
as OpenRGB names it, and none on a machine without it; a colour is `2E` 00,
`22` 1C, `30`–`32`, `36` 64, `22` 13, and the same colour again nothing;
Breathing, Flashing, Rainbow Wave, Magic, Wave, Color Cycle and Off in
OpenRGB's order; a colour into Breathing's first block, B G R; the errors; a
colour after an effect is static again; release and close write nothing; the
catalogue.

### Live (2026-11-17, OpenRGB stopped, the maintainer watching)

`sanshoku-bench` from this branch:

- `light --keys --driver msi`: "MSI GeForce RTX 4090 Suprim Liquid X
  (1462:5104)" on `/dev/i2c-3`, zone "GPU", light "Color", floor 250 ms; found
  by the adapter's name and PCI parent, nothing read from the card.
- `light --yes --driver msi --pattern steady --color 0000ff`: steady blue. The
  first frame 151 ms (six writes and their waits); the 11 after it unchanged
  and not sent.
- `onboard --effect Breathing --yes`: breathing between red and green, OpenRGB's
  starting colours (152 ms).
- `onboard --effect Breathing --colour 00a0ff --yes`: breathing between light
  blue and green.
- `onboard --effect "Rainbow Wave" --yes`: cycling through the rainbow
  (131 ms).
