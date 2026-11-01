# udev rules

The module opens device nodes as the logged-in user. `systemd-logind` grants
that user an ACL on any node tagged `uaccess`; without the tag the node is
root-only and the driver reports the device as absent with a permission error
underneath. This is the failure that is easy to leave out and impossible to
notice, because the device is found and cannot be opened.

A consumer ships the rules for the drivers it uses. The rules match by
vendor, not product, so the table below does not have to be edited in step
with a driver's allow-list; a `uaccess` tag on a device the module does not
drive costs nothing.

| Vendor | ID | Drivers | Rule |
|---|---|---|---|
| Logitech | `046d` | `logitech` | `KERNEL=="hidraw*", KERNELS=="0003:046D:*", TAG+="uaccess"` |
| Razer | `1532` | `razer` | `KERNEL=="hidraw*", KERNELS=="0003:1532:*", TAG+="uaccess"` |
| SteelSeries | `1038` | `steelseries` | `KERNEL=="hidraw*", KERNELS=="0003:1038:*", TAG+="uaccess"` |
| NZXT | `1e71` | `nzxt` | `KERNEL=="hidraw*", KERNELS=="0003:1E71:*", TAG+="uaccess"` and `SUBSYSTEM=="usb", ENV{DEVTYPE}=="usb_device", ATTR{idVendor}=="1e71", TAG+="uaccess"` |
| AULA (2.4 GHz receiver) | `3554` | `aula` | `KERNEL=="hidraw*", KERNELS=="0003:3554:*", TAG+="uaccess"` |
| AULA (F75 on its cable) | `258a` | `aula` | `KERNEL=="hidraw*", KERNELS=="0003:258A:010C.*", TAG+="uaccess"` |
| Sony (DualSense, DualSense Edge) | `054c` | `sony` | `KERNEL=="hidraw*", KERNELS=="*054C:0CE6*", TAG+="uaccess"` and the same for `0DF2` |

**Every rule matches the device itself, not a parent** (#65). In one udev
rule, an `ATTRS` key is satisfied by any ancestor of the device, so a rule
by `ATTRS{idVendor}` would tag every device plugged into that vendor's hub
or dock: a keyboard behind a Logitech dock, or, by the USB rule, any device
behind an NZXT hub. A hidraw node is matched through its HID device's name,
`0003:<vendor>:<product>.<n>` (`0003` is USB, and the hex is upper case),
which only the device has; NZXT's USB node by `DEVTYPE` and its own
`ATTR{idVendor}`. A device on Bluetooth is not matched by the USB rules,
as before.

Sony's rules match the controller by product and through its HID ID rather
than by USB vendor, for two reasons: the same vendor ID is on Sony headphones
the module does not drive, and a controller on Bluetooth has no USB parent for
`ATTRS{idVendor}` to match. They are the rules Steam's `steam-devices`
package installs, so a desk with Steam may have them already.

NZXT needs two rules because its two interfaces surface differently: status
and control as a hidraw character device, the LCD's bulk endpoint as the USB
device node itself.

Bluetooth needs no rule: L2CAP sockets and the BlueZ D-Bus API are open to
any user in the default BlueZ policy.

`packaging/60-sanshoku.rules` in this repository is the union, for a consumer
that wants all of them.
