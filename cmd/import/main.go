// Command import bulk-loads motorcycle POIs scraped from мотокарта.рф into the
// points table. The source serves its markers as Yandex ObjectManager JSON from
// GET /api/objects.php; grab that response in the browser (it 403s any non
// same-origin request and the RU host is unreachable from here) and save it to a
// file, then feed the file to this tool:
//
//	DATABASE_URL=... go run ./cmd/import data/motokarta_objects.json
//	go run ./cmd/import data/motokarta_objects.json --dry-run   # parse + report, no DB
//	go run ./cmd/import data/motokarta_objects.json --purge-imported
//
// Imported rows get created_by = NULL (no owning user), the same marker the demo
// seed migration uses. Re-running is idempotent: a row with the same name at the
// same coordinates is skipped.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"riders-connect/internal/config"
	"riders-connect/internal/database"
)

// ---------------------------------------------------------------------------
// Source format (Yandex ObjectManager)
// ---------------------------------------------------------------------------

type geometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [lat, lon] in Yandex geographic order
}

type feature struct {
	Type       string          `json:"type"`
	ID         json.RawMessage `json:"id"`
	Geometry   geometry        `json:"geometry"`
	Properties map[string]any  `json:"properties"`
}

type collection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

// parseSource accepts either a {type:"FeatureCollection", features:[...]} object
// or a bare [...] array of features.
func parseSource(raw []byte) ([]feature, error) {
	var c collection
	if err := json.Unmarshal(raw, &c); err == nil && len(c.Features) > 0 {
		return c.Features, nil
	}
	var arr []feature
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("not a FeatureCollection nor a feature array: %w", err)
	}
	return arr, nil
}

// ---------------------------------------------------------------------------
// Category mapping: source iconCaption (18 types) -> backend category (20 keys).
// "Maximize points": loosely route everything that has a sensible POI analog;
// drop the 3 non-POI / ephemeral types. Keep keys in sync with
// internal/points/models.go validCategories.
// ---------------------------------------------------------------------------

// All 18 source captions map to a backend category. "Maximize points": rider
// help/hospitality contacts (Байк-Посты / Люди / Города / Ищу компанию) go to
// cafe — the "place to stop for rest/help" bucket. The event types (Мероприятия,
// Открытие/Закрытие сезона) carry no name in the source, so they self-drop via
// the name-required rule even though they nominally map to scenic.
var categoryByCaption = map[string]string{
	"Бары":                     "bar",
	"Штабквартиры":             "bar",
	"Байк-Посты":               "cafe",
	"Люди":                     "cafe",
	"Города":                   "cafe",
	"Ищу компанию":             "cafe",
	"Место сбора":              "scenic",
	"Особенные места":          "scenic",
	"Мотомузеи":                "scenic",
	"Мероприятия":              "scenic",
	"Открытие/Закрытие сезона": "scenic",
	"Лагерь":                   "camping",
	"Мотели":                   "camping",
	"Зимнее Хранение":          "parking",
	"Сервис/Магазины":          "service",
	"Джимхана":                 "gymkhana",
	"Трасса/Трек":              "racetrack",
	"Нормативы/Соревнования":   "racetrack",
}

// ---------------------------------------------------------------------------
// Field extraction.
//
// NOTE: the exact property keys / balloon HTML shape are confirmed once a real
// objects.json is in hand. This follows the Yandex ObjectManager convention
// (balloonContentHeader = name, balloonContentBody = description + contacts).
// Adjust the key lists / regexes here after inspecting the file.
// ---------------------------------------------------------------------------

// Source balloon HTML is very regular:
//
//	balloonContentHeader: <font class='header'><img ...>{NAME} (UTC+3)</font>
//	balloonContentBody:   <font class='body'>{DESC}<br>
//	                        <div class='phone'>{contact?}<a href='tel:{PHONE}'>…</a></div>
//	                        <a href='{WEBSITE}'>Сайт</a><br>{HOURS lines}</font><hr>
//	balloonContentFooter: navigation / report-error buttons only — not an address.
var (
	anchorRe = regexp.MustCompile(`(?is)<a\b[^>]*>.*?</a>`)
	brRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	hrefHTTP = regexp.MustCompile(`(?i)href=['"](https?://[^'"]+)['"]`)
	telRe    = regexp.MustCompile(`(?i)tel:\s*([+\d][\d\s\-()]*\d)`)
	utcRe    = regexp.MustCompile(`\s*\([^()]*UTC[^()]*\)`) // any "(… UTC±N …)" timezone note
	// A line is treated as opening hours if it names a weekday / "ежедневно" /
	// "круглосуточно" or carries a HH:MM time (and is short enough not to be prose).
	schedRe = regexp.MustCompile(`(?i)(пн|вт|ср|чт|пт|сб|вс|ежедневн|круглосуточн|будни|выходн|\d{1,2}[:.]\d{2})`)
	entRepl = strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&quot;", `"`, "&#39;", "'",
		"&laquo;", "«", "&raquo;", "»", "&mdash;", "—", "&ndash;", "–")
)

func propStr(p map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// htmlLines strips tags (turning <br> and block ends into line breaks) and returns
// the non-empty trimmed text lines.
func htmlLines(s string) []string {
	s = brRe.ReplaceAllString(s, "\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = entRepl.Replace(s)
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func cleanName(header string) string {
	name := strings.Join(htmlLines(header), " ")
	name = utcRe.ReplaceAllString(name, "") // drop "(UTC+3)" / "(Лето UTC+1)" timezone notes
	return strings.TrimRight(strings.TrimSpace(name), ": ")
}

type fields struct {
	name, description, address, phone, website, hours string
}

func extractFields(p map[string]any) fields {
	var f fields
	// Name comes from the header only. Event types (Мероприятия, Открытие/Закрытие
	// сезона) carry an empty header — they're dead past festivals with no contacts —
	// so they fall out here via the name-required rule. (clusterCaption would give
	// them a junk name, so we deliberately don't fall back to it.)
	f.name = cleanName(propStr(p, "balloonContentHeader"))

	body := propStr(p, "balloonContentBody")
	if m := telRe.FindStringSubmatch(body); m != nil {
		f.phone = strings.TrimSpace(m[1])
	}
	if m := hrefHTTP.FindStringSubmatch(body); m != nil {
		f.website = m[1]
	}

	// Drop anchors (tel links + "Сайт"/"VK" labels) wholesale, keep the surrounding
	// text, then split remaining lines into description vs opening hours.
	var desc, hours []string
	for _, ln := range htmlLines(anchorRe.ReplaceAllString(body, "")) {
		if len(ln) < 60 && schedRe.MatchString(ln) {
			hours = append(hours, ln)
		} else {
			desc = append(desc, ln)
		}
	}
	f.description = strings.Join(desc, " ")
	f.hours = strings.Join(hours, "; ")
	return f
}

// ---------------------------------------------------------------------------

func validCoords(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0)
}

type stats struct {
	inserted     int
	skippedDup   int
	skippedNoCat int
	skippedNoName int
	skippedCoord int
	byCategory   map[string]int
	unknownCaps  map[string]int
}

func newStats() *stats {
	return &stats{
		byCategory:  map[string]int{},
		unknownCaps: map[string]int{},
	}
}

func main() {
	dryRun := flag.Bool("dry-run", false, "parse and report, write nothing")
	purge := flag.Bool("purge-imported", false, "delete existing created_by IS NULL points before importing")
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatalf("usage: import [--dry-run] [--purge-imported] <objects.json>")
	}
	raw, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalf("read %s: %v", flag.Arg(0), err)
	}
	feats, err := parseSource(raw)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}
	log.Printf("parsed %d features from %s", len(feats), flag.Arg(0))

	var db *sql.DB
	if !*dryRun {
		cfg := config.Load()
		db = database.Connect(cfg.DatabaseURL)
		defer db.Close()
		if *purge {
			res, err := db.Exec(`DELETE FROM points WHERE created_by IS NULL`)
			if err != nil {
				log.Fatalf("purge: %v", err)
			}
			n, _ := res.RowsAffected()
			log.Printf("purged %d existing imported points", n)
		}
	}

	st := newStats()
	for _, ft := range feats {
		caption := strings.TrimSpace(propStr(ft.Properties, "iconCaption"))
		cat, ok := categoryByCaption[caption]
		if !ok {
			st.unknownCaps[caption]++
			st.skippedNoCat++
			continue
		}
		f := extractFields(ft.Properties)
		if f.name == "" {
			st.skippedNoName++
			continue
		}
		if len(ft.Geometry.Coordinates) < 2 {
			st.skippedCoord++
			continue
		}
		lat, lon := ft.Geometry.Coordinates[0], ft.Geometry.Coordinates[1]
		if !validCoords(lat, lon) {
			st.skippedCoord++
			continue
		}

		if *dryRun {
			st.inserted++
			st.byCategory[cat]++
			continue
		}

		var exists bool
		if err := db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM points WHERE lower(name)=lower($1) AND abs(lat-$2)<0.00001 AND abs(lon-$3)<0.00001)`,
			f.name, lat, lon,
		).Scan(&exists); err != nil {
			log.Fatalf("dedup check (%q): %v", f.name, err)
		}
		if exists {
			st.skippedDup++
			continue
		}
		if _, err := db.Exec(`
			INSERT INTO points (created_by, category, name, description, lat, lon, address, phone, website, hours, photos)
			VALUES (NULL, $1, $2, $3, $4, $5, $6, $7, $8, $9, '[]'::jsonb)`,
			cat, f.name, f.description, lat, lon, f.address, f.phone, f.website, f.hours,
		); err != nil {
			log.Fatalf("insert %q: %v", f.name, err)
		}
		st.inserted++
		st.byCategory[cat]++
	}

	report(st, *dryRun)
}

func report(st *stats, dry bool) {
	verb := "inserted"
	if dry {
		verb = "would insert"
	}
	fmt.Printf("\n=== import summary ===\n%s: %d\n", verb, st.inserted)
	fmt.Printf("skipped: dup=%d no-category=%d no-name=%d bad-coords=%d\n",
		st.skippedDup, st.skippedNoCat, st.skippedNoName, st.skippedCoord)

	fmt.Println("\nby category:")
	for _, kv := range sortedCounts(st.byCategory) {
		fmt.Printf("  %-12s %d\n", kv.k, kv.v)
	}
	if len(st.unknownCaps) > 0 {
		fmt.Println("\nUNKNOWN iconCaption (not mapped — review & extend categoryByCaption):")
		for _, kv := range sortedCounts(st.unknownCaps) {
			fmt.Printf("  %-26s %d\n", kv.k, kv.v)
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
