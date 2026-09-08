package service

import (
	"context"
	"fmt"
)

// BoardEnrichmentConfig holds the cycle-B analysis configuration for a board.
// See design.md §11, decision ④ (fields live on SemanticLabel / semantic_labels table).
type BoardEnrichmentConfig struct {
	EnrichmentEnabled bool     `json:"enrichment_enabled"` // default false
	WindowDays        int      `json:"window_days"`        // default 14
	ContextLayers     []string `json:"context_layers"`     // default ["week","month","year","all"]
	AllowedTools      []string `json:"allowed_tools"`      // tools permitted for this board (from board_data_sources)
	// RelationAutoDiscoveryEnabled — per-board switch for automatic relation
	// discovery after new briefs (default false; manual trigger is always
	// available). add-evidence-backed-cross-board-relations.
	RelationAutoDiscoveryEnabled bool `json:"relation_auto_discovery_enabled"`
}

// BoardConfigReader looks up a board's enrichment configuration from a topic ID.
// The production implementation queries board_persistent_topics → semantic_labels.
// Mocks satisfy this interface for TDD without a database.
type BoardConfigReader interface {
	GetBoardConfig(ctx context.Context, topicID uint) (*BoardEnrichmentConfig, error)
}

// ── Default config ──────────────────────────────────────────────────────────

// DefaultBoardConfig returns the default enrichment config for a board.
func DefaultBoardConfig() *BoardEnrichmentConfig {
	return &BoardEnrichmentConfig{
		EnrichmentEnabled: false,
		WindowDays:        14,
		ContextLayers:     []string{"week", "month", "year", "all"},
		// RelationAutoDiscoveryEnabled defaults to false: automatic discovery
		// is opt-in per board (spec: 自动发现默认关闭).
		RelationAutoDiscoveryEnabled: false,
	}
}

// ToolsForSourceType maps a board_data_sources.source_type to the concrete tool
// names available for that source type. Built-in source types were all removed
// when the A-share financial direction was deleted, so this now always returns
// nil — web_search / fetch_page / the internal navigation tools are always-on
// (see orchestrator.explorationToolNames), not gated by source_type. The
// mechanism is retained as an extension point for future structured external
// sources.
func ToolsForSourceType(sourceType string) []string {
	return nil
}

// Validate returns an error if the config is semantically invalid.
func (c *BoardEnrichmentConfig) Validate() error {
	if c.WindowDays < 1 || c.WindowDays > 365 {
		return fmt.Errorf("window_days must be 1-365, got %d", c.WindowDays)
	}
	for _, layer := range c.ContextLayers {
		switch layer {
		case "week", "month", "year", "all":
		default:
			return fmt.Errorf("unknown context_layer: %s", layer)
		}
	}
	return nil
}
