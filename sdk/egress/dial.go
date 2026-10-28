package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// Hook uji; hanya diisi lewat export_test.go sehingga tidak dapat dipakai dari luar paket.
var (
	testDialHook  func(ctx context.Context, network, addr string) (net.Conn, error)
	testAllowAddr func(netip.AddrPort) bool
)

// blockedAddr menerapkan blockedReason kecuali hook uji mengizinkan alamat itu.
func blockedAddr(ap netip.AddrPort) (string, bool) {
	if testAllowAddr != nil && testAllowAddr(ap) {
		return "", false
	}
	return blockedReason(ap.Addr())
}

// dialContext me-resolve host, memvalidasi SEMUA IP hasilnya (gagal tertutup pada jawaban campuran),
// lalu dial berurutan ke IP tervalidasi tanpa resolve ulang (menutup DNS rebinding).
func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("egress: port tidak valid %q", portStr)
	}

	var ips []netip.Addr
	if ip, perr := netip.ParseAddr(host); perr == nil {
		ips = []netip.Addr{ip}
	} else {
		ips, err = c.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("egress: host %q tidak punya alamat", host)
	}
	for i, ip := range ips {
		ip = ip.Unmap()
		ips[i] = ip
		if reason, bad := blockedAddr(netip.AddrPortFrom(ip, uint16(port))); bad {
			return nil, &BlockedError{Host: host, Addr: ip, Reason: reason}
		}
	}

	dial := testDialHook
	if dial == nil {
		d := &net.Dialer{Timeout: c.cfg.ConnectTimeout}
		dial = d.DialContext
	}
	var errs []error
	for _, ip := range ips {
		conn, derr := dial(ctx, network, netip.AddrPortFrom(ip, uint16(port)).String())
		if derr == nil {
			return conn, nil
		}
		errs = append(errs, derr)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errs...)
}
