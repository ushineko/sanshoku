/*
Package bluez lists connected Bluetooth devices through the BlueZ ObjectManager
on the system bus, and is the driver for any device exposing
org.bluez.Battery1. Spec 004.

On Windows it lists the same devices from the properties the Bluetooth stack
keeps in the device tree, where a headset's battery level is too. Spec 016.
*/
package bluez
