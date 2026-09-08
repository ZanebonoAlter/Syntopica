package service

import (
	"strconv"

	"syntopica-backend/internal/platform/logging"
)

// ── evidence 两级分类扩展（design D4，tasks 3.5 / M3）─────────────────────────
//
// source_type（来源）扩 lane；kind（形态）新增可选两级分类 quote/series/chart。
// 校验矩阵见 test-cases M3：lane 证据 lane_id MUST 属本板块活跃集合（幽灵引用
// 拒绝且不牵连其余证据）；非法 kind 按空处理不炸报告；旧行为零回归。

// validEvidenceKinds is the closed kind enum (D4). Empty = legacy behaviour.
var validEvidenceKinds = map[string]bool{
	"quote":  true,
	"series": true,
	"chart":  true,
}

// normalizeEvidenceKind returns k when it is a legal kind, "" otherwise
// (M3.7: degrade the kind, never the evidence).
func normalizeEvidenceKind(k string) string {
	if validEvidenceKinds[k] {
		return k
	}
	if k != "" {
		logging.Warnf("evidence: unknown kind %q treated as empty (kept evidence)", k)
	}
	return ""
}

// sanitizeEvidenceChain validates a parsed evidence chain against the board's
// active lane set. Rules (M3):
//   - lane entries: lane_id must parse and belong to activeLanes; missing or
//     foreign lane_id drops THAT entry only (ghost reference, M3.5/M3.6)
//   - illegal source_type drops the whole entry (M3.8, legacy behaviour)
//   - illegal kind normalizes to "" (M3.7)
//
// activeLanes == nil (single-lane scope) rejects every lane entry — lane is a
// board-scope source type.
func sanitizeEvidenceChain(chain []EvidenceChainItem, activeLanes map[uint]bool) []EvidenceChainItem {
	if len(chain) == 0 {
		return chain
	}
	out := make([]EvidenceChainItem, 0, len(chain))
	for _, e := range chain {
		switch e.SourceType {
		case "news", "web", "page":
			e.Kind = normalizeEvidenceKind(e.Kind)
			out = append(out, e)
		case "lane":
			laneID, err := strconv.ParseUint(e.Ref, 10, 64)
			if err != nil || activeLanes == nil || !activeLanes[uint(laneID)] {
				logging.Warnf("evidence: lane ref %q not in active lane set — dropped (ghost reference)", e.Ref)
				continue
			}
			e.Kind = normalizeEvidenceKind(e.Kind)
			out = append(out, e)
		default:
			logging.Warnf("evidence: illegal source_type %q — entry dropped", e.SourceType)
			continue
		}
	}
	return out
}

// SanitizeEvidenceChainForTest exposes the sanitizer to the external test package.
func SanitizeEvidenceChainForTest(chain []EvidenceChainItem, activeLanes map[uint]bool) []EvidenceChainItem {
	return sanitizeEvidenceChain(chain, activeLanes)
}
