package egress

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
)

// Hook khusus test; tidak terekspor dari paket.

func setDialHook(f func(ctx context.Context, network, addr string) (net.Conn, error)) {
	if f == nil {
		testDialHook.Store(nil)
		return
	}
	testDialHook.Store(&f)
}

func setAllowAddrForTest(f func(netip.AddrPort) bool) {
	if f == nil {
		testAllowAddr.Store(nil)
		return
	}
	testAllowAddr.Store(&f)
}

func configOf(c *Client) Config { return c.cfg }

func proxyOf(c *Client) func(*http.Request) (*url.URL, error) { return c.transport.Proxy }
