package database

import _ "embed"

// insideAmericaMethodologyProfile is the frozen v2 snapshot of the reference
// role methodology profile (source of truth for future revisions:
// docs/research/board-analysis-reference-role/inside-america-methodology-profile.md).
// Seeded once by migration 20260826_0002; the DB row is user-owned afterwards.
//
//go:embed seeddata/inside-america-methodology-profile.md
var insideAmericaMethodologyProfile string
