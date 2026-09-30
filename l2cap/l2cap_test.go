package l2cap

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
An address is read in the order it is written.

SockaddrL2 reverses it on the way to the kernel. Reversing it here as well
dials an address nothing answers on, and the kernel calls that ECONNREFUSED —
which reads like the device declining rather than like a wrong number.
*/
func TestAnAddressIsReadInTheOrderItIsWritten(t *testing.T) {
	// Six distinct bytes, none of them anybody's: the whole point is that the
	// order is preserved, so an address that read the same backwards would
	// assert nothing.
	addr, err := ParseAddress("A1:B2:C3:D4:E5:F6")
	require.NoError(t, err)
	assert.Equal(t, [6]byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6}, addr)
}

// A string that is not an address is an error, and the error does not repeat
// the string: a malformed address is still most of somebody's address.
func TestANonAddressIsAnErrorThatDoesNotRepeatIt(t *testing.T) {
	someones := regexp.MustCompile(`(?i)[0-9a-f]{2}(:[0-9a-f]{2}){2,}`)
	for _, s := range []string{"not an address", "A1:B2:C3:D4:E5:ZZ", "A1:B2:C3:D4:E5", "A1:B2:C3:D4:E5:F6:07", "A1:B2:C3:D4:E5:F"} {
		_, err := ParseAddress(s)
		require.Error(t, err, "%q was read as an address", s)
		assert.NotRegexp(t, someones, err.Error())
		assert.NotContains(t, err.Error(), s)
	}
}
