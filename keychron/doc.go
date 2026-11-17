/*
Package keychron is the driver for a Keychron keyboard's lights over VIA and
Keychron's own RGB commands: the K4 HE first (spec 028).

Its lights are a lighting.Canvas that holds its frame. The firmware takes
each key's hue and saturation and ignores its value: brightness is one value
for the whole board, which the driver sets from the frame's brightest key. A
key cannot be dark beside a lit one. docs/keychron.md is the protocol.
*/
package keychron
