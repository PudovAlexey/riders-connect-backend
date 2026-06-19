// Command photos backfills the points.photos array with images scraped from a
// public image-search engine (DuckDuckGo by default). For each point that has no
// photos it searches for "<name> <address>", downloads up to N matching images
// into UPLOAD_DIR, and appends their public URLs to the point.
//
// IMPORTANT: image-search results are "what looks similar", NOT verified photos of
// the place — relevance varies a lot. Always preview first:
//
//	go run ./cmd/photos --dry-run --limit 15      # writes photos-preview.html, no DB writes
//	go run ./cmd/photos --limit 5                 # real run on 5 points
//	go run ./cmd/photos --reset --category cafe   # wipe photos for a selection and retry
//
// Files land in UPLOAD_DIR and the stored URLs use UPLOAD_BASE_URL, so run it with
// the same env as the server whose map you want populated (prod env for
// motocade.ru — otherwise the URLs won't resolve there).
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"riders-connect/internal/config"
	"riders-connect/internal/database"
	"riders-connect/internal/imagesearch"
	"riders-connect/internal/media"
)

const (
	minImageBytes = 5 << 10 // 5 KB — below this it's almost certainly an icon/spacer
	maxImageBytes = 8 << 20 // 8 MB
	minDimension  = 200     // px, shortest side
	browserUA     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

type pointRow struct {
	id, name, address, category string
}

// candidate carries a point and its top search hits for the dry-run preview.
type candidate struct {
	point   pointRow
	results []imagesearch.Result
}

func main() {
	dryRun := flag.Bool("dry-run", false, "search only; write photos-preview.html, no downloads or DB writes")
	reset := flag.Bool("reset", false, "set photos='[]' for the selection and exit (undo a previous run)")
	overwrite := flag.Bool("overwrite", false, "include points that already have photos")
	limit := flag.Int("limit", 0, "max points to process (0 = no limit)")
	category := flag.String("category", "", "only this point category (e.g. cafe, service)")
	n := flag.Int("n", 3, "photos to attach per point")
	engineName := flag.String("engine", "ddg", "image search engine: ddg|yandex|bing")
	delay := flag.Duration("delay", 1500*time.Millisecond, "pause between points (politeness / anti-rate-limit)")
	flag.Parse()

	cfg := config.Load()
	db := database.Connect(cfg.DatabaseURL)
	defer db.Close()

	if *reset {
		doReset(db, *category, *limit)
		return
	}

	engine, err := imagesearch.New(*engineName)
	if err != nil {
		log.Fatal(err)
	}
	points, err := selectPoints(db, *category, *overwrite, *limit)
	if err != nil {
		log.Fatalf("select points: %v", err)
	}
	log.Printf("selected %d points (engine=%s, n=%d, dry-run=%v)", len(points), engine.Name(), *n, *dryRun)

	ctx := context.Background()
	st := &stats{byCategory: map[string]int{}}
	var preview []candidate
	consecutiveErrs := 0

	for i, p := range points {
		st.processed++
		if utf8.RuneCountInString(p.name) < 3 {
			st.skippedShort++
			continue
		}
		query := buildQuery(p)

		results, err := engine.Search(ctx, query, maxInt(*n*3, 10))
		if err != nil {
			log.Printf("[%d/%d] search %q: %v", i+1, len(points), query, err)
			st.searchErr++
			consecutiveErrs++
			if consecutiveErrs >= 5 {
				log.Printf("aborting: %d consecutive search failures (likely rate-limited / CAPTCHA — try --engine, a bigger --delay, or smaller batches)", consecutiveErrs)
				break
			}
			sleep(ctx, *delay)
			continue
		}
		consecutiveErrs = 0
		if len(results) == 0 {
			st.notFound++
			log.Printf("[%d/%d] %q: no results", i+1, len(points), query)
			sleep(ctx, *delay)
			continue
		}

		if *dryRun {
			top := results
			if len(top) > *n {
				top = top[:*n]
			}
			preview = append(preview, candidate{point: p, results: top})
			st.withPhotos++
			st.byCategory[p.category]++
			sleep(ctx, *delay)
			continue
		}

		urls := downloadPhotos(ctx, results, *n, cfg.UploadDir, cfg.UploadBaseURL)
		if len(urls) == 0 {
			st.notFound++
			log.Printf("[%d/%d] %q: results found but none downloadable", i+1, len(points), query)
			sleep(ctx, *delay)
			continue
		}
		if err := updatePhotos(db, p.id, urls); err != nil {
			log.Fatalf("update %s: %v", p.id, err)
		}
		st.withPhotos++
		st.photosTotal += len(urls)
		st.byCategory[p.category]++
		log.Printf("[%d/%d] %q ← %d photo(s)", i+1, len(points), p.name, len(urls))
		sleep(ctx, *delay)
	}

	if *dryRun {
		const path = "photos-preview.html"
		if err := writePreview(path, preview, engine.Name()); err != nil {
			log.Fatalf("write preview: %v", err)
		}
		log.Printf("wrote %s — open it in a browser to review candidates before a real run", path)
	}
	report(st, *dryRun)
}

// buildQuery is "<name> <address>" — address is empty for most imported points, so
// in practice the query is usually just the name.
func buildQuery(p pointRow) string {
	q := strings.TrimSpace(p.name)
	if a := strings.TrimSpace(p.address); a != "" {
		q += " " + a
	}
	return q
}

func selectPoints(db *sql.DB, category string, overwrite bool, limit int) ([]pointRow, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT id, name, address, category FROM points WHERE TRUE`)
	var args []any
	if !overwrite {
		sb.WriteString(` AND jsonb_array_length(photos) = 0`)
	}
	if category != "" {
		args = append(args, category)
		fmt.Fprintf(&sb, ` AND category = $%d`, len(args))
	}
	sb.WriteString(` ORDER BY created_at DESC`)
	if limit > 0 {
		args = append(args, limit)
		fmt.Fprintf(&sb, ` LIMIT $%d`, len(args))
	}
	rows, err := db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pointRow
	for rows.Next() {
		var p pointRow
		if err := rows.Scan(&p.id, &p.name, &p.address, &p.category); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// doReset blanks photos for points that currently have them, honoring --category
// and --limit. Useful for re-running after a bad batch.
func doReset(db *sql.DB, category string, limit int) {
	var cond strings.Builder
	cond.WriteString(`jsonb_array_length(photos) > 0`)
	var args []any
	if category != "" {
		args = append(args, category)
		fmt.Fprintf(&cond, ` AND category = $%d`, len(args))
	}
	q := `UPDATE points SET photos='[]'::jsonb, updated_at=NOW() WHERE ` + cond.String()
	if limit > 0 {
		args = append(args, limit)
		q = `UPDATE points SET photos='[]'::jsonb, updated_at=NOW() WHERE id IN (` +
			`SELECT id FROM points WHERE ` + cond.String() + fmt.Sprintf(` LIMIT $%d)`, len(args))
	}
	res, err := db.Exec(q, args...)
	if err != nil {
		log.Fatalf("reset: %v", err)
	}
	n, _ := res.RowsAffected()
	log.Printf("reset photos on %d point(s)", n)
}

// downloadPhotos fetches candidates in order, keeping up to n valid, de-duplicated
// images saved into uploadDir, and returns their public URLs.
func downloadPhotos(ctx context.Context, results []imagesearch.Result, n int, uploadDir, baseURL string) []string {
	hc := &http.Client{Timeout: 25 * time.Second}
	var urls []string
	seen := map[[32]byte]bool{}
	for _, r := range results {
		if len(urls) >= n {
			break
		}
		data, err := fetchImage(ctx, hc, r.ImageURL)
		if err != nil {
			continue
		}
		h := sha256.Sum256(data)
		if seen[h] || !validImage(data) {
			continue
		}
		publicURL, _, err := media.SaveBytes(uploadDir, baseURL, data)
		if err != nil {
			continue
		}
		seen[h] = true
		urls = append(urls, publicURL)
	}
	return urls
}

func fetchImage(ctx context.Context, hc *http.Client, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) < minImageBytes || len(data) > maxImageBytes {
		return nil, fmt.Errorf("size out of range: %d bytes", len(data))
	}
	return data, nil
}

// validImage checks the bytes are a real image of a reasonable size. webp has no
// stdlib decoder, so when DecodeConfig fails we fall back to the sniffed MIME type
// (the dimension check is then skipped).
func validImage(data []byte) bool {
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return true
	}
	return cfg.Width >= minDimension && cfg.Height >= minDimension
}

func updatePhotos(db *sql.DB, id string, urls []string) error {
	b, err := json.Marshal(urls)
	if err != nil {
		return err
	}
	// Pass the JSON as a string so Postgres coerces it to jsonb (a []byte param
	// would be sent as bytea, which has no implicit cast to jsonb).
	_, err = db.Exec(`UPDATE points SET photos=$2, updated_at=NOW() WHERE id=$1`, id, string(b))
	return err
}

func writePreview(path string, cands []candidate, engineName string) error {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>photos preview</title>")
	b.WriteString("<style>body{font-family:sans-serif;margin:24px;background:#fafafa}" +
		"section{margin-bottom:24px;border-bottom:1px solid #ddd;padding-bottom:14px}" +
		"img{height:150px;border-radius:8px;margin:0 8px 8px 0;object-fit:cover;vertical-align:top;background:#eee}" +
		".cat{color:#888;font-size:13px;margin-left:6px}</style>")
	fmt.Fprintf(&b, "<h1>Превью фото (engine=%s) — %d точек</h1>", html.EscapeString(engineName), len(cands))
	b.WriteString("<p>Это кандидаты, которые подтянутся при реальном прогоне. Картинки грузятся напрямую из источника — часть может не открыться или оказаться нерелевантной.</p>")
	for _, c := range cands {
		b.WriteString("<section>")
		fmt.Fprintf(&b, "<div><b>%s</b><span class=cat>%s</span></div>", html.EscapeString(c.point.name), html.EscapeString(c.point.category))
		for _, r := range c.results {
			src := r.ThumbURL
			if src == "" {
				src = r.ImageURL
			}
			fmt.Fprintf(&b, "<img loading=lazy src=%q alt=%q>", src, html.EscapeString(c.point.name))
		}
		b.WriteString("</section>")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

// ---------------------------------------------------------------------------

type stats struct {
	processed    int
	withPhotos   int
	photosTotal  int
	notFound     int
	skippedShort int
	searchErr    int
	byCategory   map[string]int
}

func report(st *stats, dry bool) {
	verb := "attached photos to"
	if dry {
		verb = "would attach photos to"
	}
	fmt.Printf("\n=== photos summary ===\n")
	fmt.Printf("processed:            %d\n", st.processed)
	fmt.Printf("%s: %d points", verb, st.withPhotos)
	if !dry {
		fmt.Printf(" (%d files)", st.photosTotal)
	}
	fmt.Printf("\nno usable images:     %d\n", st.notFound)
	fmt.Printf("skipped (short name): %d\n", st.skippedShort)
	fmt.Printf("search errors:        %d\n", st.searchErr)
	if len(st.byCategory) > 0 {
		fmt.Println("\nby category:")
		for _, kv := range sortedCounts(st.byCategory) {
			fmt.Printf("  %-14s %d\n", kv.k, kv.v)
		}
	}
}

type kv struct {
	k string
	v int
}

func sortedCounts(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].v > out[j].v })
	return out
}

func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
