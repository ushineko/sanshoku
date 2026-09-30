/*
Package usbfs claims a USB interface through /dev/bus/usb and writes to a
bulk endpoint with raw usbdevfs ioctls. No libusb, no cgo. Linux only.
Spec 001.
*/
package usbfs
