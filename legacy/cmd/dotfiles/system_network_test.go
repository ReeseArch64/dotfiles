package main

import (
	"net"
	"testing"
)

func testInterfaceAddress(t *testing.T, name, cidr string) ifaceAddr {
	t.Helper()
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	network.IP = ip
	return ifaceAddr{name: name, net: network}
}

func TestDefaultRouteSelectsLowestMetricActiveInterface(t *testing.T) {
	routes := `Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
podman0 00000000 00000000 0000 0 0 0 00000000 0 0 0
wlan0 00000000 0101A8C0 0003 0 0 600 00000000 0 0 0
enp4s0 00000000 0101A8C0 0003 0 0 100 00000000 0 0 0
`
	if got := defaultRouteInterface(routes); got != "enp4s0" {
		t.Fatalf("interface da rota padrão: %q", got)
	}
}

func TestSubnetForLANPrefersDefaultRouteOverContainerNetwork(t *testing.T) {
	addrs := []ifaceAddr{
		testInterfaceAddress(t, "podman0", "10.88.0.1/16"),
		testInterfaceAddress(t, "wlan0", "192.168.1.103/24"),
	}
	if got := subnetForLAN(addrs, "wlan0"); got != "192.168.1.0/24" {
		t.Fatalf("LAN incorreta: %s", got)
	}
}
