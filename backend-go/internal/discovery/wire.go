// Package discovery is the public facade for the discovery domain
// (feed discovery / recommendation / candidate catalog / RSSHub routes),
// migrated out of internal/admin in decouple-backend-domains.
//
// Cross-domain consumers (admin/scheduler jobs, app runtime, admin handlers)
// import ONLY this root package — never discovery/{service,handler,models}.
package discovery

import (
	"syntopica-backend/internal/discovery/service"
)

// Service constructors and knobs consumed via the facade.
var (
	NewCandidateCheckService      = service.NewCandidateCheckService
	NewCandidateEmbeddingService  = service.NewCandidateEmbeddingService
	NewCatalogSyncService         = service.NewCatalogSyncService
	NewPreferenceProfileService   = service.NewPreferenceProfileService
	MarkStaleRunningDiscoveryRuns = service.MarkStaleRunningDiscoveryRuns
	LoadDiscoveryV2Enabled        = service.LoadDiscoveryV2Enabled
)

const (
	// CandidateCheckDefaultBatchSize is the default availability-check batch size.
	CandidateCheckDefaultBatchSize = service.CandidateCheckDefaultBatchSize
	// CandidateEmbeddingBatchSizeDefault is the default embedding backfill batch size.
	CandidateEmbeddingBatchSizeDefault = service.CandidateEmbeddingBatchSizeDefault
	// DiscoveryRunStaleThresholdDefault is how long a running run may sit before stale-marking.
	DiscoveryRunStaleThresholdDefault = service.DiscoveryRunStaleThresholdDefault
)
