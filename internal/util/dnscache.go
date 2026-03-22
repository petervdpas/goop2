package util

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type DNSCache struct {
	Host       string
	ResolveTTL time.Duration
	CacheTTL   time.Duration

	mu      sync.RWMutex
	ip      string
	expires time.Time
}

func NewDNSCache(host string, resolveTimeout, cacheTTL time.Duration) *DNSCache {
	return &DNSCache{
		Host:       host,
		ResolveTTL: resolveTimeout,
		CacheTTL:   cacheTTL,
	}
}

func (d *DNSCache) Resolve(ctx context.Context) (string, error) {
	if d.Host == "" || net.ParseIP(d.Host) != nil {
		return d.Host, nil
	}

	d.mu.RLock()
	if d.ip != "" && time.Now().Before(d.expires) {
		ip := d.ip
		d.mu.RUnlock()
		return ip, nil
	}
	d.mu.RUnlock()

	resolveCtx, cancel := context.WithTimeout(ctx, d.ResolveTTL)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(resolveCtx, d.Host)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", d.Host, err)
	}

	chosen := ips[0]
	for _, ip := range ips {
		if net.ParseIP(ip) != nil && net.ParseIP(ip).To4() != nil {
			chosen = ip
			break
		}
	}

	d.mu.Lock()
	d.ip = chosen
	d.expires = time.Now().Add(d.CacheTTL)
	d.mu.Unlock()

	log.Printf("dns: resolved %s → %s", d.Host, chosen)
	return chosen, nil
}

func (d *DNSCache) Ready() bool {
	if d.Host == "" || net.ParseIP(d.Host) != nil {
		return true
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ip != "" && time.Now().Before(d.expires)
}

func (d *DNSCache) Clear() {
	d.mu.Lock()
	d.ip = ""
	d.expires = time.Time{}
	d.mu.Unlock()
}

func (d *DNSCache) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	if host != d.Host {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	ip, err := d.Resolve(context.Background())
	if err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip, port))
	if err != nil {
		d.Clear()
		return nil, err
	}
	return conn, nil
}
