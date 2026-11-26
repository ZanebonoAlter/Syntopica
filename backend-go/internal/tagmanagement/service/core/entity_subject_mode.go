package core

import (
	"github.com/spf13/viper"
)

// Entity-subject shadow-mode switch (normalize-event-subject-entities D2/D10).
//
// The first-round extraction call requests the stable short identity
// description (subject_desc) ONLY in shadow mode. In off mode (the default)
// the first-round prompt, schema and call count stay BYTE-IDENTICAL to the
// pre-change behavior — the branch below selects the very same prompt/schema
// builder functions, and the regression test in extractor_section_test.go
// pins them against a frozen golden string.

// entitySubjectModeOverride lets tests force the mode without touching viper
// (nil → resolve from viper config). NEVER set in production code.
var entitySubjectModeOverride *string

// SetEntitySubjectModeForTest forces the entity-subject mode inside this
// process (extractor tests). Pass "" to hand control back to viper.
func SetEntitySubjectModeForTest(mode string) {
	if mode == "" {
		entitySubjectModeOverride = nil
		return
	}
	m := mode
	entitySubjectModeOverride = &m
}

// EntitySubjectShadowEnabled reports whether first-round extraction runs in
// shadow mode (adds subject_desc to prompt+schema; still ONE call per
// section). off / invalid values → false (byte-identical legacy behavior).
func EntitySubjectShadowEnabled() bool {
	if entitySubjectModeOverride != nil {
		return *entitySubjectModeOverride == "shadow"
	}
	return viper.GetString("entity_subject.mode") == "shadow"
}
