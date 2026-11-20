/*
Package gigabyte is the driver for Gigabyte RGB Fusion 2 USB mainboard
controllers: the Z790 AORUS MASTER X's two first (spec 039).

Each controller is a lighting.Canvas and a lighting.Zoned: its addressable
headers, whose length the board cannot read and a consumer sets, and its
single-colour lights. The controller does not say which lights it has, so a
board's layout comes from a table of measured boards, keyed by the board's DMI
name and the controller's number; a board not in it is refused. Names follow
OpenRGB's, so that what was written against OpenRGB's listing still finds
them. docs/gigabyte-fusion2.md is the protocol.
*/
package gigabyte
