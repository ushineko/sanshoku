//go:build !linux

package hidraw

// SplitsReceivers says whether the system gives each device paired to a
// receiver a node of its own. Off Linux it does not: everything paired to a
// receiver is reached through the receiver or not at all (spec 012).
const SplitsReceivers = false
