/*
Package support is the hardware support table: one Entry per device or
protocol a driver speaks, with the tier it has reached (tested on named
hardware, expected to work by protocol, or listed and not implemented).
Each driver exports its entries; all.Support() collects them; docs/devices.md
is generated from that. It imports nothing from this module.
*/
package support
