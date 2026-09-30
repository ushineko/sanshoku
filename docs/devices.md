# Supported devices

One row per device the module has been run against, or per protocol it
speaks by specification. "Measured" means the bench has produced the numbers
on the named hardware and the spec records the output; "by protocol" means
the code path exists because the protocol says so, and no device has
confirmed it.

| Device | IDs | Bus | Driver | Capability | Status | Spec |
|---|---|---|---|---|---|---|
| Intel CPU package (`coretemp`) | – | hwmon | `hwmon` | temperature | measured | 001 |
| AMD CPU (`k10temp` Tdie/Tctl, `zenpower`) | – | hwmon | `hwmon` | temperature | measured (hayami) | 001 |
| AMD GPU (`amdgpu` edge), nouveau | – | hwmon | `hwmon` | temperature | measured (hotaru) | 001 |
| Logitech G502 X PLUS via Lightspeed | `046d:*` by descriptor | hidraw | `logitech` | battery | pending | 002 |
| Logitech K800 via Unifying (HID++ 1.0) | `046d:*` by descriptor | hidraw | `logitech` | battery | pending | 002 |
| Any HID++ 2.0 device with feature 0x1004 or 0x1000 | `046d:*` | hidraw | `logitech` | battery | by protocol | 002 |
| Razer Mouse Dock Pro (mouse behind it) | `1532:00a4` | hidraw | `razer` | battery | pending | 003 |
| Razer Mouse Dock, Basilisk Ultimate dongle | `1532:007e`, `1532:0088` | hidraw | `razer` | battery | by protocol | 003 |
| SteelSeries Apex Pro TKL Wireless Gen 3 | `1038:1644`, `1038:1646` | hidraw | `steelseries` | battery | pending | 003 |
| SteelSeries Aerox 3/5/9, Prime Wireless | see spec 003 | hidraw | `steelseries` | battery | by protocol | 003 |
| SteelSeries Rival 3 Wireless, Rival 3 Gen 2, Rival 650 | see spec 003 | hidraw | `steelseries` | – | listed, not implemented | 003 |
| Apple AirPods (AAP, per ear and case) | BlueZ Modalias `v004C` | L2CAP | `apple` | battery | pending | 004 |
| Any BlueZ device with `Battery1` | – | D-Bus | `bluez` | battery | by protocol | 004 |
| NZXT Kraken Elite, firmware 1.2.0 | `1e71:3012` | hidraw + usbfs | `nzxt` | cooling, screen | pending | 005 |

Out of scope: every lit device (OpenRGB, the standing exception), NVIDIA
temperature (nvidia-smi; NVML is a vendor library), fan and pump duty writes
on the Kraken (the firmware discards them).

Candidates for a later spec, by the rule that direct access is preferred
wherever reasonable: SteelSeries Arctis Nova Pro battery (today via
`headsetcontrol`; it sits on the 0xFFC0 usage page the `steelseries` driver
already scans), and the SteelSeries legacy 0xAA protocol once a Rival is on
the desk.
