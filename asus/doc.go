/*
Package asus is the driver for an ASUS Aura USB mainboard controller: the ROG
Maximus Z790 Hero's first (spec 027).

Its lights are a lighting.Canvas and a lighting.Zoned: the board's own lights,
and an addressable header per strip, whose length the board cannot read and a
consumer sets. Names follow OpenRGB's, so that what was written against
OpenRGB's listing still finds them. docs/asus-aura.md is the protocol.
*/
package asus
