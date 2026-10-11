/*
Package aula is the driver for an AULA keyboard, the F75.

Through its 2.4 GHz receiver it reads the keyboard's battery, which the
receiver answers for on a vendor report (spec 013), and draws on its lights by
streaming frames (spec 019). On its cable the keyboard is another device, with
the lights and no battery reading.

The lights are a lighting.Canvas that is also a lighting.Fading: the firmware
takes them back a moment after frames stop, so a still frame is sent again to
be kept. Through the receiver a frame shows at most ReceiverColours colours at
once; on the cable every key has its own.
*/
package aula
