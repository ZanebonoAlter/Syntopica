// Package sources implements the per-source fetchers. Parsing contracts are
// translated 1:1 from tools/energy-mcp/sources.py (the calibrated spec, see
// docs/research/research-assistant-data-sources/explore-findings.md): en-dash
// pairs → null, thousand separators stripped, missing markers never become 0,
// structural drift → SCHEMA_CHANGED with no fuzzy matching.
package sources

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// cp1252Decode decodes a CP1252 byte payload (EIA WPSR emits 0x96 en-dash).
func cp1252Decode(b []byte) (string, error) {
	out, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

const enDash = "–" // CP1252 0x96

// parseNumber parses a numeric CSV cell. Returns (value, hasValue, missing).
//   - missing != "" → a known missing marker (en-dash pair / empty cell)
//   - hasValue → a finite number ("0" stays 0.0; NaN/Inf/overflow rejected)
//   - !hasValue && missing == "" → neither numeric nor a known marker: caller
//     MUST raise SCHEMA_CHANGED (unknown drift).
func parseNumber(cell string) (float64, bool, string) {
	raw := strings.TrimSpace(cell)
	if raw == "" {
		return 0, false, "empty cell"
	}
	if strings.TrimSpace(strings.Trim(raw, enDash)) == "" {
		return 0, false, "en-dash missing marker"
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", ""), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, ""
	}
	return v, true, ""
}

var footnoteRe = regexp.MustCompile(`^\(\d+\)\s*`)

// normalizeLabel strips a leading footnote like "(1)     " and collapses
// internal whitespace (B-section labels carry trailing spaces + footnote
// numbers per the energy-mcp field notes).
func normalizeLabel(raw string) string {
	s := footnoteRe.ReplaceAllString(raw, "")
	return strings.Join(strings.Fields(s), " ")
}

var mdyRe = regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{2}$`)

// parseMDY parses "8/28/26" into 2026-08-28 (yy>=70 pivots to 1900s).
// Returns "" when the token is not an M/D/YY date.
func parseMDY(token string) string {
	t := strings.TrimSpace(token)
	if !mdyRe.MatchString(t) {
		return ""
	}
	parts := strings.Split(t, "/")
	month, _ := strconv.Atoi(parts[0])
	day, _ := strconv.Atoi(parts[1])
	yy, _ := strconv.Atoi(parts[2])
	year := 1900 + yy
	if yy < 70 {
		year = 2000 + yy
	}
	d := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if d.Month() != time.Month(month) || d.Day() != day { // e.g. 13/45/26
		return ""
	}
	return d.Format("2006-01-02")
}

// isMDYHeader reports whether a header cell is an M/D/YY date column.
func isMDYHeader(cell string) bool { return mdyRe.MatchString(strings.TrimSpace(cell)) }
