package tools

// fetch.go implements fetch_url — an HTTP(S)-only fetch tool with an SSRF
// guard applied at *dial time* via a custom Transport.DialContext, not just
// a pre-check against the parsed hostname. A pre-check-then-dial approach is
// vulnerable to DNS rebinding (the hostname resolves to a public IP at
// check-time and a private/loopback IP at dial-time); validating the IP the
// connection actually opens against closes that gap.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// maxFetchBody caps the response body read into the tool result.
const maxFetchBody = 1024 * 1024 // 1 MiB

// isBlockedIP reports whether ip must not be dialed by fetch_url: loopback,
// private, link-local, and unspecified ranges — the standard SSRF blocklist
// for a tool that fetches an agent/LLM-supplied URL.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	var lastErr error
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			lastErr = fmt.Errorf("refusing to dial %s: resolves to a blocked (loopback/private/link-local) address %s", host, ip)
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable address for %q", host)
	}
	return nil, lastErr
}

var fetchClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext: guardedDialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
}

type fetchArgs struct {
	URL string `json:"url" jsonschema:"The http:// or https:// URL to fetch."`
}

type fetchResult struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
	Truncated   bool   `json:"truncated"`
	Message     string `json:"message"`
}

func fetchURL(_ tool.Context, args fetchArgs) (fetchResult, error) {
	if !strings.HasPrefix(args.URL, "http://") && !strings.HasPrefix(args.URL, "https://") {
		return fetchResult{}, fmt.Errorf("only http:// and https:// URLs are supported, got %q", args.URL)
	}

	req, err := http.NewRequest(http.MethodGet, args.URL, nil)
	if err != nil {
		return fetchResult{}, fmt.Errorf("build request for %q: %w", args.URL, err)
	}

	resp, err := fetchClient.Do(req)
	if err != nil {
		return fetchResult{}, fmt.Errorf("fetch %q: %w", args.URL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxFetchBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fetchResult{}, fmt.Errorf("read response body for %q: %w", args.URL, err)
	}
	truncated := len(body) > maxFetchBody
	if truncated {
		body = body[:maxFetchBody]
	}

	slog.Info("tool_call", "kind", "ToolCall", "tool", "fetch_url", "url", args.URL, "status", resp.StatusCode, "bytes", len(body))

	msg := fmt.Sprintf("Fetched %q — status %d, %d bytes.", args.URL, resp.StatusCode, len(body))
	if truncated {
		msg += fmt.Sprintf(" Truncated at %d bytes.", maxFetchBody)
	}

	return fetchResult{
		URL:         args.URL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        string(body),
		Truncated:   truncated,
		Message:     msg,
	}, nil
}

// NewFetchURLTool creates the fetch_url FunctionTool.
func NewFetchURLTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "fetch_url",
		Description: "Fetches an http:// or https:// URL and returns its status, content type, and body (capped at 1 MiB). Refuses to dial loopback, private, or link-local addresses (SSRF guard), including after DNS resolution.",
	}, fetchURL)
	if err != nil {
		panic(fmt.Sprintf("NewFetchURLTool: %v", err))
	}
	return t
}
