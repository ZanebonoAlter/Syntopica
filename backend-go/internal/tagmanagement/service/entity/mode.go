// Package entity implements the global entity-subject identity layer and the
// subject-mention resolution chain (normalize-event-subject-entities):
//
//	first-round extraction (shadow adds subject_desc, same call)
//	  → three candidate paths merged & deduped by entity_id
//	  → second-round PURE choice question (legal entity_key or none)
//	  → atomic register (short tx) / reuse, behind a cross-process gate
//
// Technical failure and legal `none` are STRICTLY distinct outcomes; neither
// the event-story identity path nor any business table is read or written by
// the decision itself (shadow writes are limited to the entity layer, normal
// AI audit and the topic_tags.metadata.entity_subject lazy key).
package entity

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Mode is the entity-subject switch (design D10).
type Mode string

const (
	// ModeOff — default. First-round prompt/schema/call count byte-identical,
	// zero second-round calls, zero entity-layer writes, no entity_subject key.
	ModeOff Mode = "off"
	// ModeShadow — first round additionally produces the stable short identity
	// description; writes limited to entity layer + AI audit + the lazy
	// metadata.entity_subject key.
	ModeShadow Mode = "shadow"
	// ModeActive — NOT implemented this iteration. Resolving to active is an
	// EXPLICIT error (never silently behaves as shadow); production
	// enablement waits for the bounded evaluation verdict.
	ModeActive Mode = "active"
)

// ErrModeActiveNotImplemented is returned when the configured mode is
// `active`: production normalization is deliberately not wired this
// iteration and must fail loudly instead of quietly running as shadow.
var ErrModeActiveNotImplemented = errors.New("entity_subject.mode=active 本期未实现：生产启用需待有界评测达标后另开 change；不会静默按 shadow 执行")

// ModeFromValue validates a raw mode string. Empty → off: this is the safe
// fallback for binaries/tests that never load platform/config, so an absent
// knob can never silently start writing to the entity layer. The effective
// production default is `off`, set by platform/config's
// viper.SetDefault("entity_subject.mode", "off") (2026-11-10 decision: the
// shadow experiment ended, the entity layer is not wired into the event-line
// decision path; shadow stays available through configuration).
// Unknown values → off with an error (misconfiguration must be visible, not
// silent shadow). active → ErrModeActiveNotImplemented (explicit rejection).
func ModeFromValue(raw string) (Mode, error) {
	switch Mode(raw) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeShadow:
		return ModeShadow, nil
	case ModeActive:
		return ModeActive, ErrModeActiveNotImplemented
	default:
		return ModeOff, fmt.Errorf("entity_subject.mode 非法值 %q（合法值 off|shadow；active 本期显式拒绝），回退 off", raw)
	}
}

// CurrentMode resolves the mode from viper config (entity_subject.mode,
// default off set by platform/config). Invalid values degrade to off; active
// is reported as ModeActive so callers decide how to surface the explicit
// rejection (the resolver treats ModeActive as a hard error).
func CurrentMode() (Mode, error) {
	return ModeFromValue(viper.GetString("entity_subject.mode"))
}
