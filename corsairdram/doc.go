/*
Package corsairdram is the driver for Corsair's RGB memory modules, over the
chipset's SMBus: the Dominator Platinum RGB DDR5's first (spec 042).

Each module is a lighting.Canvas of its LEDs in a line, drawn by direct
frames. A module is found by two identification registers and named by its
device-information block, as OpenRGB finds and names it; the driver writes
only the registers that read that block and take frames, and never one that
switches the module to its bootloader or writes its configuration.
docs/corsair-dram.md is the protocol. Linux only, as package i2c is.
*/
package corsairdram
