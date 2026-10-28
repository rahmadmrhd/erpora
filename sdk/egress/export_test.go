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
	testDialHook = f
}

func setAllowAddrForTest(f func(netip.AddrPort) bool) { testAllowAddr = f }

func configOf(c *Client) Config { return c.cfg }

func proxyOf(c *Client) func(*http.Request) (*url.URL, error) { return c.transport.Proxy }
