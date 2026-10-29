/*
Package powersupply reads the batteries the kernel itself reads for a HID
device, from /sys/class/power_supply. Spec 022.

A kernel HID driver that knows a device's battery publishes it there:
hid-nintendo for a Switch Pro Controller and the pads that pose as one,
hid-playstation for a DualSense, xpadneo for an Xbox pad over Bluetooth,
hid-input for any device whose descriptor declares a battery strength. Some of
those devices BlueZ has no battery for, a classic Bluetooth gamepad among
them, so without this package nothing in the module found them.

The driver is a sanshoku.Fallback: where another driver reads the same device
directly, sanshoku.Scan lists that driver's candidate and not this one's.
Linux only; elsewhere Find finds nothing.
*/
package powersupply
