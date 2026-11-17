/*
Package corsair is the driver for Corsair's "V2" peripherals: the MM700 RGB
mousepad first (spec 026).

Its lights are a lighting.Canvas that holds its frame. The pad shows a frame
only in software mode, and a frame sent in hardware mode is accepted and not
shown, so the driver reads the mode before each frame and claims software mode
when it has to. Release hands the pad back to its stored effect.
docs/corsair-v2.md is the protocol.
*/
package corsair
