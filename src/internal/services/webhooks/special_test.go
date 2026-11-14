package webhooks

import (
	"net"
	"testing"
)

// Every IANA special-purpose range, and every way of writing a loopback or
// private address that is not a plain literal, is refused both when an
// endpoint is added and when it is dialled (review R-SEC-06: 0.0.0.0/8,
// 198.18.0.0/15, 240.0.0.0/4, NAT64, 6to4, IPv4-compatible and site-local
// addresses all passed as public).
func TestSpecialPurposeAddressesAreNotPublic(t *testing.T) {
	refused := []string{
		"0.0.0.1", "0.1.2.3", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.0.1",
		"192.0.0.8", "192.0.2.1", "192.88.99.1", "192.168.1.1", "198.18.0.1", "198.19.255.255",
		"198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255",
		"::", "::1", "::ffff:127.0.0.1", "::127.0.0.1", "::a00:1",
		"64:ff9b::7f00:1", "64:ff9b::a00:1", "64:ff9b:1::a00:1", "2002:7f00:1::1", "2002:a00:1::1",
		"2001::1", "2001:db8::1", "100::1", "fc00::1", "fd12:3456::1", "fe80::1", "fec0::1", "ff02::1",
	}
	for _, a := range refused {
		host := a
		if net.ParseIP(a).To4() == nil {
			host = "[" + a + "]"
		}
		if err := CheckURL("https://"+host+"/hook", false); err == nil {
			t.Errorf("CheckURL accepted %s", a)
		}
		if err := checkDial(net.JoinHostPort(a, "443")); err == nil {
			t.Errorf("the dialer would connect to %s", a)
		}
	}
	// Names that are addresses in disguise are refused when added; the
	// dialer sees what they resolve to.
	for _, h := range []string{"localhost.", "LOCALHOST", "api.localhost.", "2130706433", "0x7f.1", "017700000001", "127.1"} {
		if err := CheckURL("https://"+h+"/hook", false); err == nil {
			t.Errorf("CheckURL accepted host %q", h)
		}
	}
	// Public addresses, and NAT64/6to4 forms of public IPv4, still pass.
	for _, a := range []string{"93.184.215.14", "2606:4700::6810:84e5", "64:ff9b::5db8:d70e", "2002:5db8:d70e::1"} {
		host := a
		if net.ParseIP(a).To4() == nil {
			host = "[" + a + "]"
		}
		if err := CheckURL("https://"+host+"/hook", false); err != nil {
			t.Errorf("CheckURL refused public %s: %v", a, err)
		}
		if err := checkDial(net.JoinHostPort(a, "443")); err != nil {
			t.Errorf("the dialer refused public %s: %v", a, err)
		}
	}
	if err := CheckURL("https://hooks.example.com/x", false); err != nil {
		t.Errorf("a plain name was refused: %v", err)
	}
}
