/*
Package msi is the driver for an MSI graphics card's lighting controller over
I2C: the GeForce RTX 4090 Suprim Liquid X first (spec 029).

The card has one light. Its colour is a lighting.Canvas, held as a static
colour; its own effects are lighting.Onboard and lighting.Catalogue, since the
card cannot be animated from the host faster than a few frames a second.
Every register write is followed by 20 ms, and the card needs time in its
static mode between colours. docs/msi-gpu.md is the protocol.
*/
package msi
