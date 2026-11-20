/*
Package evga is the driver for the lighting controller of EVGA's Ampere
graphics cards, over I2C: the RTX 3080 FTW3 Ultra LHR's first (spec 041).

The card's lights are a lighting.Canvas: each of its zones (the front logo,
the end-plate logo, the back logo and the addressable header) shows one
colour, as the controller allows no per-LED colour. Names follow OpenRGB's.
docs/evga-gpu.md is the protocol. Linux only, as package i2c is.
*/
package evga
