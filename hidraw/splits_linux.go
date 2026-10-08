package hidraw

// SplitsReceivers says whether the system gives each device paired to a
// receiver a node of its own. Linux does: hid-logitech-dj presents a Unifying
// receiver's keyboard as a node beside the receiver's (see PairedChild), and
// a driver reading through the receiver node can leave a paired device to its
// own. Windows has no such driver, and everything paired to a receiver is
// reached through the receiver or not at all (spec 012).
const SplitsReceivers = true
