package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewClient returns an HTTP client that refuses to connect to private,
// loopback, link-local or multicast addresses, protecting against SSRF both at
// dial time (after DNS resolution) and across redirects.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           SafeDialContext,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return ValidateURL(req.URL)
		},
	}
}

func ValidateURL(u *url.URL) error {
	if u == nil {
		return errors.New("url is required")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must use http or https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return errors.New("url must have a host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return errors.New("url must not point to a local hostname")
	}
	if ip := net.ParseIP(host); ip != nil && isDisallowedIP(ip) {
		return errors.New("url must not point to a private or loopback address")
	}
	return nil
}

func ValidateURLString(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	return ValidateURL(u)
}

func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		if isDisallowedIP(ip.IP) {
			lastErr = fmt.Errorf("blocked target address %s", ip.IP)
			continue
		}
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no address available for target")
	}
	return nil, lastErr
}

func isDisallowedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() ||
		ip.IsInterfaceLocalMulticast() {
		return true
	}
	return strings.HasPrefix(ip.String(), "100.64.")
}
