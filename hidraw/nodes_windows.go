package hidraw

import (
	"slices"
	"strings"
)

/*
Nodes lists the HID interfaces of one vendor that satisfy want. A vendor of
zero means any vendor; a nil want takes every node.

**One node per interface, not per collection.** Windows gives each top-level
collection a device path of its own, and a protocol does not respect that
split: a Logitech receiver's HID++ interface carries its short report in one
collection and its long report in another, and a short request is answered
with a long report. The collections are put back together under the device
instance they hang off, which for a USB device is the interface, so a driver
sees what it sees on Linux: one node that asks and answers.

Never by product ID and never by position, for the reasons the Linux Nodes
gives. A collection that cannot be described -- one that went away between
the listing and the look, or one the system will not open even with no
access -- is skipped, as an unreadable hidraw node is.
*/
func Nodes(vendor uint16, want func(Node) bool) ([]Node, error) {
	paths, err := interfaces()
	if err != nil {
		return nil, err
	}
	var infos []collectionInfo
	for _, p := range paths {
		info, err := describe(p)
		if err != nil || (vendor != 0 && info.vendor != vendor) {
			continue
		}
		infos = append(infos, info)
	}
	var found []Node
	for _, n := range group(infos) {
		if want != nil && !want(n) {
			continue
		}
		found = append(found, n)
	}
	return found, nil
}

/*
group puts collections back together into one Node per device instance, in
a stable order: by the first collection's path, so the same desk lists the same
way twice.

The node's Path is its first collection's, which is enough for Open to find
the rest; it is a collection path and not the instance because an instance can
carry a device's serial and a path is printed.
*/
func group(infos []collectionInfo) []Node {
	byParent := map[string][]collectionInfo{}
	var order []string
	for _, info := range infos {
		key := info.parent
		if key == "" {
			// A collection whose parent could not be found stands alone.
			key = info.path
		}
		if _, ok := byParent[key]; !ok {
			order = append(order, key)
		}
		byParent[key] = append(byParent[key], info)
	}
	nodes := make([]Node, 0, len(order))
	for _, key := range order {
		members := byParent[key]
		slices.SortFunc(members, func(a, b collectionInfo) int { return strings.Compare(a.path, b.path) })
		first := members[0]
		n := Node{
			Path:    first.path,
			Name:    undouble(strings.TrimSpace(first.manufacturer + " " + first.name)),
			Phys:    first.parent,
			Vendor:  first.vendor,
			Product: first.product,
			Bus:     busOf(first.parent),
		}
		for _, m := range members {
			n.Reports = append(n.Reports, m.reports...)
		}
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(a, b Node) int { return strings.Compare(a.Path, b.Path) })
	return nodes
}

// busOf reads the bus from the enumerator at the front of a device instance:
// USB for `USB\`, Bluetooth for the classic and LE enumerators. Anything else
// is reported as USB, which is where every HID device this module reads is.
func busOf(instance string) uint16 {
	upper := strings.ToUpper(instance)
	if strings.HasPrefix(upper, `BTHENUM\`) || strings.HasPrefix(upper, `BTHLEDEVICE\`) ||
		strings.HasPrefix(upper, `BTHLE\`) {
		return BusBluetooth
	}
	return BusUSB
}

/*
reportsFromCaps turns a collection's capabilities into Reports: one per kind,
report ID, page and usage, each with the collection's length for that kind.

**A length with no capabilities behind it is still a report.** A Razer mouse's
interface 0 declares a 91-byte feature report and Windows lists no capability
for it (measured, spec 012): the report is real, it is the one the battery is
read through, and leaving it out would make the node look as if it had no
feature report at all. It is recorded as unnumbered, on the collection's own
page and usage, because a collection that numbered its reports would have
capabilities that say so.
*/
func reportsFromCaps(info collectionInfo, read func(kind uintptr, button bool) []hidCap) []Report {
	type key struct {
		kind  ReportKind
		id    byte
		page  uint16
		usage uint16
	}
	kinds := []struct {
		kind  ReportKind
		hidp  uintptr
		bytes int
	}{
		{ReportInput, hidpInput, info.inLen},
		{ReportOutput, hidpOutput, info.outLen},
		{ReportFeature, hidpFeature, info.featLen},
	}
	var out []Report
	seen := map[key]bool{}
	for _, k := range kinds {
		if k.bytes == 0 {
			continue
		}
		caps := append(read(k.hidp, false), read(k.hidp, true)...)
		if len(caps) == 0 {
			caps = []hidCap{{UsagePage: info.page, Usage: info.usage}}
		}
		for _, c := range caps {
			id := key{k.kind, c.ReportID, c.UsagePage, c.Usage}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, Report{Kind: k.kind, ID: c.ReportID, Page: c.UsagePage, Usage: c.Usage, Len: k.bytes})
		}
	}
	return out
}
