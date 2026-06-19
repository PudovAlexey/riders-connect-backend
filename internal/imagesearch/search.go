// Package imagesearch fetches candidate image URLs for a text query by scraping
// public image-search engines. There is no official, key-free photo API for the
// places on the map, so this screen-scrapes DuckDuckGo / Yandex / Bing results.
//
// That is inherently fragile: the engines can change their markup, rate-limit, or
// serve a CAPTCHA at any time. When that happens Search returns an error or an
// empty slice and the caller simply leaves the point without photos.
package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Result is one candidate image found for a query.
type Result struct {
	ImageURL   string // full-size image
	ThumbURL   string // smaller preview (may equal ImageURL or be empty)
	SourcePage string // page the image was found on (may be empty)
}

// Engine searches one provider for images matching a text query.
type Engine interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
	Name() string
}

// New returns the engine for the given name: "ddg" (default), "yandex", "bing".
func New(name string) (Engine, error) {
	c := &client{hc: &http.Client{Timeout: 20 * time.Second}}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "ddg", "duckduckgo":
		return &ddg{c}, nil
	case "yandex":
		return &yandex{c}, nil
	case "bing":
		return &bing{c}, nil
	default:
		return nil, fmt.Errorf("unknown engine %q (want ddg|yandex|bing)", name)
	}
}

// ---------------------------------------------------------------------------
// shared HTTP client
// ---------------------------------------------------------------------------

type client struct{ hc *http.Client }

// get fetches url with browser-like headers, retrying once on a transient
// failure (network error, 429, or 5xx). The body is capped at 6 MB.
func (c *client) get(ctx context.Context, target, referer string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,*/*;q=0.8")
		req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.8")
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
		}
		return body, nil
	}
	return nil, lastErr
}

// decodeURL turns a URL extracted from HTML/JSON back into a plain URL: it undoes
// JSON slash-escaping and HTML entity encoding.
func decodeURL(s string) string {
	s = strings.NewReplacer(`\/`, `/`).Replace(s)
	return html.UnescapeString(s)
}

// ---------------------------------------------------------------------------
// DuckDuckGo (default) — two-step: page for the vqd token, then the i.js JSON API.
// ---------------------------------------------------------------------------

type ddg struct{ c *client }

func (d *ddg) Name() string { return "ddg" }

var ddgVQD = regexp.MustCompile(`vqd=['"]?([\d-]+)['"]?`)

func (d *ddg) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	page, err := d.c.get(ctx, "https://duckduckgo.com/?q="+url.QueryEscape(query)+"&iax=images&ia=images", "")
	if err != nil {
		return nil, fmt.Errorf("vqd page: %w", err)
	}
	m := ddgVQD.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("vqd token not found (DuckDuckGo markup changed or CAPTCHA)")
	}

	api := "https://duckduckgo.com/i.js?l=ru-ru&o=json&f=,,,&p=1&q=" + url.QueryEscape(query) + "&vqd=" + string(m[1])
	body, err := d.c.get(ctx, api, "https://duckduckgo.com/")
	if err != nil {
		return nil, fmt.Errorf("i.js: %w", err)
	}
	var payload struct {
		Results []struct {
			Image     string `json:"image"`
			Thumbnail string `json:"thumbnail"`
			URL       string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode i.js: %w", err)
	}
	out := make([]Result, 0, limit)
	for _, r := range payload.Results {
		if r.Image == "" {
			continue
		}
		out = append(out, Result{ImageURL: r.Image, ThumbURL: r.Thumbnail, SourcePage: r.URL})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Yandex Images — better RU relevance, but aggressive CAPTCHA. Scrapes the
// "img_href" fields embedded in the SERP HTML (both raw and HTML-encoded forms).
// ---------------------------------------------------------------------------

type yandex struct{ c *client }

func (y *yandex) Name() string { return "yandex" }

var (
	yandexImgEnc = regexp.MustCompile(`&quot;img_href&quot;:&quot;(https?:[^&]+?)&quot;`)
	yandexImgRaw = regexp.MustCompile(`"img_href":"(https?:\\?/\\?/[^"]+?)"`)
)

func (y *yandex) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	page, err := y.c.get(ctx, "https://yandex.ru/images/search?text="+url.QueryEscape(query)+"&isize=large", "")
	if err != nil {
		return nil, err
	}
	out := collectURLs(page, limit, yandexImgEnc, yandexImgRaw)
	if len(out) == 0 {
		return nil, fmt.Errorf("no images parsed (Yandex markup changed or CAPTCHA)")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Bing Images — scrapes the "murl" (media URL) fields from the result anchors.
// ---------------------------------------------------------------------------

type bing struct{ c *client }

func (b *bing) Name() string { return "bing" }

var (
	bingMurlEnc = regexp.MustCompile(`&quot;murl&quot;:&quot;(.*?)&quot;`)
	bingMurlRaw = regexp.MustCompile(`"murl":"(.*?)"`)
)

func (b *bing) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	page, err := b.c.get(ctx, "https://www.bing.com/images/search?q="+url.QueryEscape(query)+"&form=HDRSC2&first=1", "")
	if err != nil {
		return nil, err
	}
	out := collectURLs(page, limit, bingMurlEnc, bingMurlRaw)
	if len(out) == 0 {
		return nil, fmt.Errorf("no images parsed (Bing markup changed or CAPTCHA)")
	}
	return out, nil
}

// collectURLs runs each regex over the page in order, decoding and de-duplicating
// the first capture group, until limit unique image URLs are gathered.
func collectURLs(page []byte, limit int, res ...*regexp.Regexp) []Result {
	seen := map[string]bool{}
	var out []Result
	for _, re := range res {
		for _, m := range re.FindAllSubmatch(page, -1) {
			u := decodeURL(string(m[1]))
			if u == "" || seen[u] || !strings.HasPrefix(u, "http") {
				continue
			}
			seen[u] = true
			out = append(out, Result{ImageURL: u, ThumbURL: u})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}
