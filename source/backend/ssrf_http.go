package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type publicClientOptions struct {
	Timeout            time.Duration
	NoTotalTimeout     bool
	LocalIP            string
	AllowInsecureTLS   bool
	MaxRedirects       int
	AllowHTTP          bool
	DialTimeout        time.Duration
	DisableCompression bool
}

// validatePublicHTTPURL verifies HTTP/HTTPS syntax, disallows credentials and
// resolves hostnames to ensure no loopback, link-local, RFC1918, ULA, or multicast IPs.
func validatePublicHTTPURL(ctx context.Context, rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return errors.New("URL is empty")
	}
	if err := validateDownloadURL(trimmed); err != nil {
		return err
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
	}
	return validatePublicDownloadHost(ctx, trimmed)
}

// newPublicHTTPClient creates an http.Client whose DialContext verifies every
// resolved destination IP against isPublicDownloadIP and whose CheckRedirect
// blocks non-HTTP(S), embedded credentials, excessive hops, and private IPs.
func newPublicHTTPClient(opts publicClientOptions) *http.Client {
	timeout := opts.Timeout
	if opts.NoTotalTimeout {
		timeout = 0
	} else if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 6 * time.Second
	}
	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 5
	}

	var localAddr net.Addr
	if ip := net.ParseIP(strings.TrimSpace(opts.LocalIP)); ip != nil {
		localAddr = &net.TCPAddr{IP: ip}
	}

	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 15 * time.Second,
		LocalAddr: localAddr,
	}

	tr := &http.Transport{
		Proxy:                 nil, // Never silently pivot via ambient proxy env vars to internal hosts
		ForceAttemptHTTP2:     false,
		DisableCompression:    opts.DisableCompression,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid target address %q: %w", addr, err)
			}
			if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
				if !isPublicDownloadIP(ip) {
					return nil, fmt.Errorf("blocked private or reserved destination IP %s", ip.String())
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
			}
			if err := validateResolvedDownloadAddresses(ips); err != nil {
				return nil, err
			}

			var lastErr error
			for _, candidate := range ips {
				if !isPublicDownloadIP(candidate.IP) {
					continue
				}
				if localTCP, ok := localAddr.(*net.TCPAddr); ok && localTCP.IP != nil {
					localIsV4 := localTCP.IP.To4() != nil
					candIsV4 := candidate.IP.To4() != nil
					if localIsV4 != candIsV4 {
						continue
					}
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no reachable public IP family matched for %s", host)
		},
	}

	if opts.AllowInsecureTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if req.URL == nil {
				return errors.New("redirect missing URL")
			}
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "https" && !(opts.AllowHTTP && scheme == "http") {
				return fmt.Errorf("blocked redirect to scheme %q", scheme)
			}
			if req.URL.User != nil {
				return errors.New("blocked redirect URL containing credentials")
			}
			if err := validateDownloadURL(req.URL.String()); err != nil {
				return err
			}
			return validatePublicDownloadHost(req.Context(), req.URL.String())
		},
	}
}

// sanitizeTelegramError removes Telegram bot token substrings from errors and URLs.
func sanitizeTelegramError(token string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	trimmed := strings.TrimSpace(token)
	if trimmed != "" {
		msg = strings.ReplaceAll(msg, trimmed, "[REDACTED_TOKEN]")
		msg = strings.ReplaceAll(msg, url.QueryEscape(trimmed), "[REDACTED_TOKEN]")
		msg = strings.ReplaceAll(msg, url.PathEscape(trimmed), "[REDACTED_TOKEN]")
	}
	// Also strip generic /bot<id>:<secret> patterns in net/url error strings
	for {
		idx := strings.Index(msg, "/bot")
		if idx < 0 {
			break
		}
		rest := msg[idx+4:]
		slash := strings.IndexAny(rest, "/ \"'&\r\n")
		var seg string
		if slash >= 0 {
			seg = rest[:slash]
		} else {
			seg = rest
		}
		if strings.Contains(seg, ":") && len(seg) > 12 {
			msg = msg[:idx] + "/bot[REDACTED_TOKEN]" + rest[len(seg):]
		} else {
			break
		}
	}
	return msg
}

// redactSensitiveLines redacts credential-like lines from UCI/config dumps
// sent to UI views or LLM system prompts.
func redactSensitiveLines(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(lower, "option key ") ||
			strings.Contains(lower, ".key=") ||
			strings.Contains(lower, "option password ") ||
			strings.Contains(lower, ".password=") ||
			strings.Contains(lower, "option secret ") ||
			strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "bot_token") ||
			strings.Contains(lower, "token=") {
			if eq := strings.Index(line, "="); eq >= 0 {
				out = append(out, line[:eq+1]+"'***REDACTED***'")
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[0] == "option" {
				out = append(out, fmt.Sprintf("\toption %s '***REDACTED***'", fields[1]))
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
