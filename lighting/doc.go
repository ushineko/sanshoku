/*
Package lighting holds the Canvas capability for a device whose lights a
program draws frame by frame: the addressable keys, one frame, the hand-back
to the firmware and the frame floor. It imports nothing from this module and
touches no kernel interface.

There are no effects here. A consumer owns the renderer and the ticker; the
capability owns the device's facts.
*/
package lighting
