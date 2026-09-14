package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
	readersvc "syntopica-backend/internal/reader/service"
)

// ── 候选可用性检查服务（improve-discovery-recommendations 3.3，design D7 / spec C6）──
//
// 检查 = 「解析实际端点 → 有界安全抓取（safefetch：逐跳 DNS/IP 校验、无环境代理）→
// 可解析 RSS/Atom 才算过 → 状态机（availability.go）→ upsert candidate_availability」。
//
// 契约要点：
//   - access_scope=private_pending（导入/新增时标注的私有或含凭据来源，尚未获用户确认）
//     一律拒绝，不发任何请求；private_allowed（用户已确认）才允许发起；
//   - 模板带必填参数且无可直接验证实例 → requires_parameters，不发请求（参数缺失不是源失效）；
//   - 端点（实际地址）变化使旧检查结论作废，状态回 unknown 重新评估；
//   - 检查结果只写 candidate_availability，绝不触碰 feed_candidates.recommendation_enabled
//     与 feeds（spec C6「修复后复查」：源恢复不解除人工停用，不动已有订阅）。

// CandidateErrorCodeForbidden 表示无权探测（私有来源未经用户确认授权）。
const CandidateErrorCodeForbidden = "forbidden"

// CandidateCheckDefaultBatchSize 是到期候选批量取用的默认上限（4.6 调度分批）。
const CandidateCheckDefaultBatchSize = 50

func newCandidateForbiddenError(msg string) error {
	return &CandidateError{Code: CandidateErrorCodeForbidden, Message: msg}
}

func newCandidateConflictErrorCheckInProgress(id uint) error {
	return &CandidateError{
		Code:    CandidateErrorCodeConflict,
		Message: fmt.Sprintf("availability check already in progress for candidate %d", id),
	}
}

// CandidateFetchResult 是可用性检查所需的抓取结果：内嵌 safefetch.Result（该包为本
// change 的只读成果，直接复用其 SSRF/体积/超时/重定向保证），另带检查状态机需要的
// 重试提示。RetryAfterSeconds 仅在 429 且响应带 Retry-After 时非零。
type CandidateFetchResult struct {
	*safefetch.Result
	RetryAfterSeconds int
}

// CandidateCheckFetcher 是有界安全抓取的可替换实现（生产为 safefetch.Fetch 包装）。
type CandidateCheckFetcher func(ctx context.Context, rawURL string, opts safefetch.Options) (*CandidateFetchResult, error)

// candidateCheckFetcher 是包级可替换抓取函数（存 CandidateCheckFetcher）：生产走
// safefetch，测试注入 mock（照抄 reader/service 的 SetSubscriptionFetcher 注入模式）。
// 与后者同理用 atomic.Value：换装可能与在途检查读并发（残留 goroutine 读 +
// 下个用例 cleanup 还原写 = 数据竞争）。
var candidateCheckFetcher atomic.Value // 存 CandidateCheckFetcher

// currentCandidateCheckFetcher 取当前抓取实现；从未换装时用生产实现。
func currentCandidateCheckFetcher() CandidateCheckFetcher {
	if f, ok := candidateCheckFetcher.Load().(CandidateCheckFetcher); ok && f != nil {
		return f
	}
	return fetchCandidateAvailability
}

// SetCandidateCheckFetcher 覆盖抓取实现并返回还原函数（测试用）。f 为 nil 视为还原生产实现。
func SetCandidateCheckFetcher(f CandidateCheckFetcher) func() {
	prev := currentCandidateCheckFetcher()
	if f == nil {
		f = fetchCandidateAvailability
	}
	candidateCheckFetcher.Store(f)
	return func() { candidateCheckFetcher.Store(prev) }
}

// fetchCandidateAvailability 是生产抓取实现：委托 safefetch.Fetch（默认 Options =
// 10s 超时 / 2MiB 上限 / 最多 3 次重定向 / 私网拒绝 / 忽略环境代理），并把响应头里的
// Retry-After（delta-seconds）透出给状态机——429 按有界推迟复查（design D7），不再
// 退化为固定默认周期。
func fetchCandidateAvailability(ctx context.Context, rawURL string, opts safefetch.Options) (*CandidateFetchResult, error) {
	res, err := safefetch.Fetch(ctx, rawURL, opts)
	if err != nil {
		return nil, err
	}
	return &CandidateFetchResult{Result: res, RetryAfterSeconds: res.RetryAfterSeconds}, nil
}

// CandidateCheckResult 是一次同步检查的结果（handler 直接序列化；字段名对齐前端
// front/app/types/discovery.ts 的 availability/last_checked_at）。
// 不返回端点的完整地址（design D7：无权/失败探测不回传私有完整 URL）。
type CandidateCheckResult struct {
	CandidateID   uint       `json:"candidate_id"`
	Availability  string     `json:"availability"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastErrorCode string     `json:"last_error_code"`
	NextCheckAt   *time.Time `json:"next_check_at"`
}

// CandidateCheckService 执行候选可用性检查并持久化状态。
type CandidateCheckService struct {
	db *gorm.DB
	// now 是检查时刻来源（测试注入固定时钟，驱动「连续 3 次跨 24h」等时间窗分支）。
	now func() time.Time
	// cfg 是状态机参数（零值取 availability.go 默认）。
	cfg AvailabilityConfig
	// baseURL 解析 RSSHub 实例基址（默认读 ai_settings，测试可替换）。
	baseURL func() string
}

// NewCandidateCheckService 构造。
func NewCandidateCheckService(db *gorm.DB) *CandidateCheckService {
	return &CandidateCheckService{
		db:      db,
		now:     time.Now,
		baseURL: func() string { return resolveRSSHubBaseURL(db) },
	}
}

// CheckCandidate 对单个候选执行一次同步可用性检查（单次有界超时由 safefetch 默认
// Options 保证），返回新状态。同一候选检查进行中重复调用返回 conflict（进程内防重入）：
// 并发检查会重复累加失败计数、扭曲「连续失败跨时间窗」的升级判定，故显式拒绝而非静默并行。
//
// discovery_v2 开关关闭时直接拒绝（configuration 错误 → handler 503）：关闭 = 停检查
// 端点与后台检查任务，推荐主链不受影响（见 discovery_v2_switch.go）。
//
// opts 为 safefetch 抓取参数（零值 = 默认：10s 超时 / 2MiB / 3 次重定向 / 私网拒绝）。
// access_scope=private_allowed（用户已确认）的候选由调用方在 opts.AllowedIPs 里传入
// 该来源授权范围内的地址；未传时仍受 safefetch 默认私网拒绝约束（不放宽 SSRF 策略）。
func (s *CandidateCheckService) CheckCandidate(ctx context.Context, candidateID uint, opts safefetch.Options) (*CandidateCheckResult, error) {
	if !LoadDiscoveryV2Enabled(s.db) {
		return nil, newCandidateConfigurationError(discoveryV2DisabledMessage)
	}
	release, ok := acquireCandidateCheck(candidateID)
	if !ok {
		return nil, newCandidateConflictErrorCheckInProgress(candidateID)
	}
	defer release()

	now := s.now()
	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&cand, candidateID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newCandidateNotFoundError(candidateID)
		}
		return nil, err
	}

	// 私有来源：未经用户显式确认（private_pending）不得在自动/手动检查中发起请求。
	if cand.AccessScope == "private_pending" {
		return nil, newCandidateForbiddenError(fmt.Sprintf(
			"candidate %d is private_pending: access must be confirmed by the user before any check", candidateID))
	}

	endpoint, hasEndpoint, err := s.resolveCheckEndpoint(ctx, &cand)
	if err != nil {
		return nil, err
	}

	row, found, err := s.loadAvailability(ctx, candidateID)
	if err != nil {
		return nil, err
	}

	if !hasEndpoint {
		// 模板需必填参数且无可直接验证实例：记 requires_parameters，不发请求。
		state := MarkRequiresParameters(availabilityStateFromRow(row), now)
		saved, err := s.persistAvailability(ctx, row, state, "")
		if err != nil {
			return nil, err
		}
		return candidateCheckResult(saved), nil
	}

	state := availabilityStateFromRow(row)
	if found && row.LastEndpointKey != "" && row.LastEndpointKey != endpoint {
		// 端点变化：旧结论失效，从 unknown 重新评估（不继承旧 broken/ok 与失败计数）。
		state = AvailabilityState{Status: AvailabilityStatusUnknown}
	}

	// 私网授权接线（Medium 7）：private_allowed 候选只放行其端点解析到的那些 IP；
	// 解析失败不静默退化（不把用户已确认的放行变成默认拒绝）。
	fetchOpts, err := authorizePrivateEndpoint(ctx, cand.AccessScope, endpoint, opts)
	if err != nil {
		return nil, newCandidateValidationError(fmt.Sprintf("candidate %d private endpoint resolution failed", candidateID))
	}

	res, fetchErr := currentCandidateCheckFetcher()(ctx, endpoint, fetchOpts)
	next := Evaluate(state, classifyCheckOutcome(res, fetchErr, now), s.cfg)

	saved, err := s.persistAvailability(ctx, row, next, endpoint)
	if err != nil {
		return nil, err
	}
	return candidateCheckResult(saved), nil
}

// DueCandidateIDs 返回下次检查到期的候选 ID（next_check_at 升序，最多 limit 条），
// 供调度任务（4.6）分批取用；private_pending 恒排除，requires_parameters 不排队
// （无端点可查，等用户填参后端点变化自然重新入队）。
func (s *CandidateCheckService) DueCandidateIDs(ctx context.Context, now time.Time, limit int) ([]uint, error) {
	if limit <= 0 {
		limit = CandidateCheckDefaultBatchSize
	}
	ids := []uint{}
	err := s.db.WithContext(ctx).
		Model(&models.FeedCandidate{}).
		Joins("LEFT JOIN candidate_availability ON candidate_availability.candidate_id = feed_candidates.id").
		Where("feed_candidates.access_scope <> ?", "private_pending").
		Where("(candidate_availability.id IS NULL OR (candidate_availability.status <> ? AND (candidate_availability.next_check_at IS NULL OR candidate_availability.next_check_at <= ?)))",
			AvailabilityStatusRequiresParams, now).
		Order("candidate_availability.next_check_at ASC").
		Limit(limit).
		Pluck("feed_candidates.id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// resolveCheckEndpoint 解析候选的实际可检查端点。
// hasEndpoint=false 表示模板需必填参数且无可直接验证实例（requires_parameters）。
// 返回的 endpoint 是规范化后的地址，同时充当端点身份 key（LastEndpointKey）。
func (s *CandidateCheckService) resolveCheckEndpoint(ctx context.Context, cand *models.FeedCandidate) (string, bool, error) {
	// 候选自带显式实例地址（原生 RSS，或 RSSHub 填参后落库的直接地址）优先。
	if cand.FeedURL != nil && strings.TrimSpace(*cand.FeedURL) != "" {
		normalized, err := NormalizeRSSURL(*cand.FeedURL)
		if err != nil {
			return "", false, asCandidateValidationError(err)
		}
		return *normalized, true, nil
	}
	if cand.Kind != "rsshub" {
		return "", false, newCandidateValidationError(fmt.Sprintf("candidate %d has no feed url to check", cand.ID))
	}
	if cand.RouteID == nil {
		return "", false, newCandidateValidationError(fmt.Sprintf("candidate %d has no rsshub route", cand.ID))
	}
	var route models.RSSHubRoute
	if err := s.db.WithContext(ctx).First(&route, *cand.RouteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, newCandidateValidationError(fmt.Sprintf("candidate %d references missing route %d", cand.ID, *cand.RouteID))
		}
		return "", false, err
	}
	// 与 accept 同一条拼装规则（buildFeedURL）：usable_directly 用 example / namespace+path，
	// requires_parameters 无实例时返回空串 → requires_parameters。
	built := buildFeedURL(&route, nil, s.baseURL())
	if strings.TrimSpace(built) == "" {
		return "", false, nil
	}
	normalized, err := NormalizeRSSURL(built)
	if err != nil {
		return "", false, asCandidateValidationError(err)
	}
	return *normalized, true, nil
}

// loadAvailability 读取候选当前可用性行；无行时返回带 CandidateID 的空行（found=false）。
func (s *CandidateCheckService) loadAvailability(ctx context.Context, candidateID uint) (models.CandidateAvailability, bool, error) {
	var row models.CandidateAvailability
	err := s.db.WithContext(ctx).Where("candidate_id = ?", candidateID).First(&row).Error
	if err == nil {
		return row, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CandidateAvailability{CandidateID: candidateID}, false, nil
	}
	return row, false, err
}

// persistAvailability 落库一次检查后的状态（无行则建行，有行则只更新状态列）。
func (s *CandidateCheckService) persistAvailability(ctx context.Context, existing models.CandidateAvailability, state AvailabilityState, endpointKey string) (*models.CandidateAvailability, error) {
	row := existing
	row.CandidateID = existing.CandidateID
	row.Status = state.Status
	row.LastAttemptAt = state.LastAttemptAt
	row.LastSuccessAt = state.LastSuccessAt
	row.ConsecutiveFailures = state.ConsecutiveFailures
	row.FirstFailureAt = state.FirstFailureAt
	row.NextCheckAt = state.NextCheckAt
	row.LastErrorCode = state.LastErrorCode
	row.LastEndpointKey = endpointKey

	db := s.db.WithContext(ctx)
	if row.ID == 0 {
		if err := db.Create(&row).Error; err != nil {
			return nil, err
		}
		return &row, nil
	}
	if err := db.Model(&models.CandidateAvailability{}).Where("id = ?", row.ID).Updates(map[string]any{
		"status":               row.Status,
		"last_attempt_at":      row.LastAttemptAt,
		"last_success_at":      row.LastSuccessAt,
		"consecutive_failures": row.ConsecutiveFailures,
		"first_failure_at":     row.FirstFailureAt,
		"next_check_at":        row.NextCheckAt,
		"last_error_code":      row.LastErrorCode,
		"last_endpoint_key":    row.LastEndpointKey,
		"updated_at":           s.now(),
	}).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// availabilityStateFromRow 把持久化行还原为状态机输入（无记录/空状态按 unknown：
// 未验证既不是通过也不是失效）。
func availabilityStateFromRow(row models.CandidateAvailability) AvailabilityState {
	status := row.Status
	if status == "" {
		status = AvailabilityStatusUnknown
	}
	return AvailabilityState{
		Status:              status,
		ConsecutiveFailures: row.ConsecutiveFailures,
		FirstFailureAt:      row.FirstFailureAt,
		LastAttemptAt:       row.LastAttemptAt,
		LastSuccessAt:       row.LastSuccessAt,
		NextCheckAt:         row.NextCheckAt,
		LastErrorCode:       row.LastErrorCode,
	}
}

// candidateCheckResult 由持久化行构造对外结果。
func candidateCheckResult(row *models.CandidateAvailability) *CandidateCheckResult {
	return &CandidateCheckResult{
		CandidateID:   row.CandidateID,
		Availability:  row.Status,
		LastCheckedAt: row.LastAttemptAt,
		LastErrorCode: row.LastErrorCode,
		NextCheckAt:   row.NextCheckAt,
	}
}

// classifyCheckOutcome 把抓取结果归类为状态机输入（design D7）：
// 410 直接判确定失效；429 推迟不升级；2xx 还要过 RSS 解析（只看 HTTP 200 不算通过）；
// 其余状态码与传输层错误都是「暂时失败」，单次不判死。
func classifyCheckOutcome(res *CandidateFetchResult, err error, at time.Time) CheckOutcome {
	if err != nil || res == nil || res.Result == nil {
		return CheckOutcome{Kind: OutcomeNetworkError, At: at}
	}
	switch {
	case res.StatusCode == http.StatusGone:
		return CheckOutcome{Kind: OutcomeHTTP410, At: at}
	case res.StatusCode == http.StatusTooManyRequests:
		return CheckOutcome{Kind: OutcomeHTTP429, RetryAfterSeconds: res.RetryAfterSeconds, At: at}
	case res.StatusCode >= 200 && res.StatusCode < 300:
		if _, parseErr := readersvc.ParseFeedBody(res.Body); parseErr != nil {
			return CheckOutcome{Kind: OutcomeHTTPOkContentInvalid, At: at}
		}
		return CheckOutcome{Kind: OutcomeHTTPOkContentValid, At: at}
	default:
		return CheckOutcome{Kind: OutcomeHTTPError, At: at}
	}
}

// ── 进程内防重入（同一候选检查进行中拒绝第二次）──

var candidateCheckInFlight = struct {
	mu   sync.Mutex
	seen map[uint]struct{}
}{seen: map[uint]struct{}{}}

// acquireCandidateCheck 抢占候选的检查权；ok=false 表示已有检查在进行中。
// 返回的释放函数必须 defer 调用。
func acquireCandidateCheck(id uint) (func(), bool) {
	candidateCheckInFlight.mu.Lock()
	defer candidateCheckInFlight.mu.Unlock()
	if _, busy := candidateCheckInFlight.seen[id]; busy {
		return nil, false
	}
	candidateCheckInFlight.seen[id] = struct{}{}
	return func() {
		candidateCheckInFlight.mu.Lock()
		delete(candidateCheckInFlight.seen, id)
		candidateCheckInFlight.mu.Unlock()
	}, true
}
