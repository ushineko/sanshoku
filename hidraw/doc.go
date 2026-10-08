/*
Package hidraw finds HID interfaces, parses report descriptors, and exchanges
reports and feature reports with them. Spec 001.

On Linux it enumerates /sys/class/hidraw and opens /dev/hidrawN. On Windows
(spec 012) it reads the same things through the HID class driver: each
interface's top-level collections are put back together into one Node, a write
goes to the collection that declares its report, and a read takes the first
report from any of them. Elsewhere it builds and finds nothing.
*/
package hidraw
