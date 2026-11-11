package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var defaultSources = []string{
	"https://news.ycombinator.com/rss",
	"https://github.blog/feed/",
	"https://blog.cloudflare.com/rss/",
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
}

func allowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublic {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func allowedURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Hostname() != "" && (u.Port() == "" || u.Port() == "443")
}

func publicClient(allow func(*url.URL) bool) *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 4 || !allowedURL(r.URL) || !allow(r.URL) {
				return errors.New("destino de redirecionamento fora das permissões")
			}
			return nil
		},
		Transport: &http.Transport{
			TLSHandshakeTimeout: 6 * time.Second,
			DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil || port != "443" {
					return nil, errors.New("destino inválido")
				}
				ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
				if err != nil {
					return nil, err
				}
				d := net.Dialer{Timeout: 4 * time.Second}
				for _, ip := range ips {
					if !allowedIP(ip) {
						continue
					}
					if c, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), port)); err == nil {
						return c, nil
					}
				}
				return nil, errors.New("servidor público IPv4 indisponível")
			},
		},
	}
}

func request(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !allowedURL(u) {
		return nil, errors.New("a fonte deve ser uma URL HTTPS pública")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JarvisLocal/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err == nil && int64(len(b)) > max {
		return nil, errors.New("arquivo excede o limite")
	}
	return b, err
}

func shortened(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

var tags = regexp.MustCompile(`(?s)<[^>]+>`)

type FeedEntry struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Summary     string `xml:"summary"`
	Links       []struct {
		Value string `xml:",chardata"`
		URL   string `xml:"href,attr"`
		Rel   string `xml:"rel,attr"`
	} `xml:"link"`
}

func parseFeed(b []byte, host string) ([]Evidence, error) {
	var feed struct {
		Channel struct {
			Items []FeedEntry `xml:"item"`
		} `xml:"channel"`
		Entries []FeedEntry `xml:"entry"`
	}
	if err := xml.Unmarshal(b, &feed); err != nil {
		return nil, err
	}
	entries := feed.Channel.Items
	if len(entries) == 0 {
		entries = feed.Entries
	}
	list := []Evidence{}
	clean := func(s string, n int) string {
		return shortened(strings.Join(strings.Fields(tags.ReplaceAllString(html.UnescapeString(s), " ")), " "), n)
	}
	for _, e := range entries {
		title := clean(e.Title, 180)
		if title == "" {
			continue
		}
		summary := clean(e.Description+" "+e.Summary, 260)
		for _, l := range e.Links {
			candidate := strings.TrimSpace(l.URL + l.Value)
			u, err := url.Parse(candidate)
			if err == nil && allowedURL(u) && (l.Rel == "" || l.Rel == "alternate") {
				list = append(list, Evidence{Title: title, Summary: summary, Source: host, Link: shortened(candidate, 500)})
				break
			}
		}
		if len(list) == 3 {
			break
		}
	}
	if len(list) == 0 {
		return nil, errors.New("feed sem itens com link HTTPS")
	}
	return list, nil
}

func gather(ctx context.Context, profile, dir string) ([]Evidence, []string) {
	urls := defaultSources
	if data, err := os.ReadFile(filepath.Join(profile, "fontes.txt")); err == nil {
		urls = nil
		for _, line := range strings.Split(string(data), "\n") {
			if s := strings.TrimSpace(line); s != "" && !strings.HasPrefix(s, "#") {
				urls = append(urls, s)
			}
			if len(urls) >= 6 {
				break
			}
		}
	}
	client := publicClient(func(*url.URL) bool { return true })
	defer client.CloseIdleConnections()
	var all []Evidence
	var log []string
	for i, source := range urls {
		if ctx.Err() != nil {
			break
		}
		entries, err := downloadFeed(ctx, client, source, filepath.Join(dir, fmt.Sprintf("fonte-%d.xml", i+1)))
		if err != nil {
			log = append(log, fmt.Sprintf("Fonte %d: %s", i+1, shortened(err.Error(), 300)))
			continue
		}
		for _, e := range entries {
			e.ID = fmt.Sprintf("F%d", len(all)+1)
			all = append(all, e)
		}
		log = append(log, fmt.Sprintf("%s: %d itens", entries[0].Source, len(entries)))
	}
	return all, log
}

func downloadFeed(ctx context.Context, client *http.Client, source, path string) ([]Evidence, error) {
	resp, err := request(ctx, client, source)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	entries, err := parseFeed(b, strings.TrimPrefix(strings.ToLower(resp.Request.URL.Hostname()), "www."))
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		return nil, err
	}
	return entries, nil
}
