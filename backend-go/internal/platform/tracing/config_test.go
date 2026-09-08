package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultConfigSampleRatioLow guards the nil-AppConfig fallback path in
// cmd/server/main.go: full sampling kept otel_spans at ~4.3GB steady state
// (820k spans/day), so the fallback must stay at the low default too.
func TestDefaultConfigSampleRatioLow(t *testing.T) {
	cfg := DefaultConfig()
	require.InDelta(t, 0.05, cfg.SampleRatio, 1e-9)
}
