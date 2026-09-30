package battery

import "strings"

/*
Product is a device's name without the vendor's: "SteelSeries Arctis Nova Pro
Wireless" from a SteelSeries driver is "Arctis Nova Pro Wireless".

The kernel's HID_NAME is the manufacturer string and the product string run
together, so the vendor comes first and takes the room a consumer has for the
name. hayami draws it in a cell half a card wide and showed "SteelSeries
Arctis…": the vendor kept and the product cut. The driver already knows the
vendor, and a person reading the name beside a battery level does not need it
twice.

A leading word equal to vendor, compared case-insensitively, is dropped, with
any corporate suffix after it ("NZXT, Inc. Kraken" is "Kraken"), the same rule
hidraw applies to a doubled vendor word. A name that does not start with the
vendor is returned as it is, and so is one that is nothing but the vendor: a
device that will not say what it is called is better named after its maker
than not named at all.
*/
func Product(vendor, name string) string {
	words := strings.Fields(name)
	if len(words) < 2 || !strings.EqualFold(strings.Trim(words[0], ",."), vendor) {
		return name
	}
	i := 1
	for i < len(words) && corporate(strings.Trim(words[i], ",.")) {
		i++
	}
	if i == len(words) {
		return name
	}
	return strings.Join(words[i:], " ")
}

// corporate is a word that follows a company's name and not a product's. It is
// hidraw's list, repeated here because this package imports nothing from the
// module.
func corporate(word string) bool {
	switch strings.ToLower(word) {
	case "inc", "ltd", "co", "corp", "corporation", "gmbh", "llc", "limited":
		return true
	}
	return false
}
