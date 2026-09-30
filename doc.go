/*
Package sanshoku is the vocabulary every driver in this module shares: an
Identity, a Candidate found by a Driver, an open Device, the sentinel errors,
and Scan, which runs a set of drivers and lists what they found. Nothing here
opens a device; the transports and drivers do. See docs/design.md.
*/
package sanshoku
