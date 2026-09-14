package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// base 是固定基准时刻；各 case 用相对时间构造跨度边界。
var availBase = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func ptrAt(t time.Time) *time.Time { return &t }

// TestEvaluate 表驱动覆盖 test-cases.md S14 白盒分支：
// consecutive=0/2/3、跨度 23h59m/24h、429 有/无 Retry-After、超 7d 封顶、
// 成功重置、410 直接 broken。
func TestEvaluate(t *testing.T) {
	week := 7 * 24 * time.Hour

	tests := []struct {
		name    string
		prev    AvailabilityState
		outcome CheckOutcome
		cfg     AvailabilityConfig
		check   func(t *testing.T, got AvailabilityState)
	}{
		{
			name:    "首次成功从unknown转ok",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown},
			outcome: CheckOutcome{Kind: OutcomeHTTPOkContentValid, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusOK, got.Status)
				require.Equal(t, 0, got.ConsecutiveFailures)
				require.Nil(t, got.FirstFailureAt)
				require.Empty(t, got.LastErrorCode)
				require.NotNil(t, got.LastSuccessAt)
				require.Equal(t, availBase, *got.LastSuccessAt)
				require.NotNil(t, got.NextCheckAt)
				require.Equal(t, availBase.Add(week), *got.NextCheckAt)
				require.Equal(t, availBase, *got.LastAttemptAt)
			},
		},
		{
			name: "成功重置失败计数与首次失败时间",
			prev: AvailabilityState{
				Status: AvailabilityStatusOK, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-2 * time.Hour)),
				LastErrorCode:  OutcomeNetworkError,
			},
			outcome: CheckOutcome{Kind: OutcomeHTTPOkContentValid, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusOK, got.Status)
				require.Equal(t, 0, got.ConsecutiveFailures)
				require.Nil(t, got.FirstFailureAt)
				require.Empty(t, got.LastErrorCode)
			},
		},
		{
			name:    "单次网络错误不判死保持unknown",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown},
			outcome: CheckOutcome{Kind: OutcomeNetworkError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusUnknown, got.Status)
				require.Equal(t, 1, got.ConsecutiveFailures)
				require.NotNil(t, got.FirstFailureAt)
				require.Equal(t, availBase, *got.FirstFailureAt)
				require.Equal(t, OutcomeNetworkError, got.LastErrorCode)
				require.Equal(t, availBase.Add(week), *got.NextCheckAt)
			},
		},
		{
			name:    "ok源一次http错误保持ok",
			prev:    AvailabilityState{Status: AvailabilityStatusOK},
			outcome: CheckOutcome{Kind: OutcomeHTTPError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusOK, got.Status)
				require.Equal(t, 1, got.ConsecutiveFailures)
			},
		},
		{
			name: "三次失败但跨度仅2h不升级broken",
			prev: AvailabilityState{
				Status: AvailabilityStatusUnknown, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-2 * time.Hour)),
			},
			outcome: CheckOutcome{Kind: OutcomeNetworkError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusUnknown, got.Status)
				require.Equal(t, 3, got.ConsecutiveFailures)
			},
		},
		{
			name: "三次失败跨度23h59m仍差一分钟不破",
			prev: AvailabilityState{
				Status: AvailabilityStatusUnknown, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-23*time.Hour - 59*time.Minute)),
			},
			outcome: CheckOutcome{Kind: OutcomeHTTPError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.NotEqual(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, 3, got.ConsecutiveFailures)
			},
		},
		{
			name: "三次失败跨度恰24h升级broken",
			prev: AvailabilityState{
				Status: AvailabilityStatusUnknown, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-24 * time.Hour)),
			},
			outcome: CheckOutcome{Kind: OutcomeNetworkError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, 3, got.ConsecutiveFailures)
				require.Equal(t, availBase.Add(week), *got.NextCheckAt)
			},
		},
		{
			name: "内容无效同样计入失败并可升级broken",
			prev: AvailabilityState{
				Status: AvailabilityStatusOK, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-25 * time.Hour)),
			},
			outcome: CheckOutcome{Kind: OutcomeHTTPOkContentInvalid, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, OutcomeHTTPOkContentInvalid, got.LastErrorCode)
			},
		},
		{
			name: "仅两次失败即使跨度很长也不破",
			prev: AvailabilityState{
				Status: AvailabilityStatusUnknown, ConsecutiveFailures: 1,
				FirstFailureAt: ptrAt(availBase.Add(-48 * time.Hour)),
			},
			outcome: CheckOutcome{Kind: OutcomeNetworkError, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.NotEqual(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, 2, got.ConsecutiveFailures)
			},
		},
		{
			name:    "410无前置失败直接broken",
			prev:    AvailabilityState{Status: AvailabilityStatusOK, ConsecutiveFailures: 0},
			outcome: CheckOutcome{Kind: OutcomeHTTP410, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, 1, got.ConsecutiveFailures)
				require.Equal(t, OutcomeHTTP410, got.LastErrorCode)
				require.Equal(t, availBase.Add(week), *got.NextCheckAt)
			},
		},
		{
			name:    "429按RetryAfter推迟且不计失败",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown, ConsecutiveFailures: 0},
			outcome: CheckOutcome{Kind: OutcomeHTTP429, RetryAfterSeconds: 3600, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusUnknown, got.Status)
				require.Equal(t, 0, got.ConsecutiveFailures)
				require.Nil(t, got.FirstFailureAt)
				require.Equal(t, OutcomeHTTP429, got.LastErrorCode)
				require.Equal(t, availBase.Add(time.Hour), *got.NextCheckAt)
			},
		},
		{
			name:    "429无RetryAfter用默认周期",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown},
			outcome: CheckOutcome{Kind: OutcomeHTTP429, RetryAfterSeconds: 0, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, availBase.Add(week), *got.NextCheckAt)
			},
		},
		{
			name:    "429RetryAfter超7天封顶",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown},
			outcome: CheckOutcome{Kind: OutcomeHTTP429, RetryAfterSeconds: 30 * 24 * 3600, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, availBase.Add(7*24*time.Hour), *got.NextCheckAt)
			},
		},
		{
			name: "429不打断失败计数也不累加",
			prev: AvailabilityState{
				Status: AvailabilityStatusUnknown, ConsecutiveFailures: 2,
				FirstFailureAt: ptrAt(availBase.Add(-2 * time.Hour)),
			},
			outcome: CheckOutcome{Kind: OutcomeHTTP429, RetryAfterSeconds: 60, At: availBase},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, 2, got.ConsecutiveFailures)
				require.NotNil(t, got.FirstFailureAt)
				require.Equal(t, availBase.Add(time.Minute), *got.NextCheckAt)
			},
		},
		{
			name:    "阈值窗口可配置为更紧",
			prev:    AvailabilityState{Status: AvailabilityStatusUnknown, ConsecutiveFailures: 1, FirstFailureAt: ptrAt(availBase.Add(-time.Hour))},
			outcome: CheckOutcome{Kind: OutcomeNetworkError, At: availBase},
			cfg:     AvailabilityConfig{BrokenThreshold: 2, BrokenWindow: time.Hour},
			check: func(t *testing.T, got AvailabilityState) {
				require.Equal(t, AvailabilityStatusBroken, got.Status)
				require.Equal(t, 2, got.ConsecutiveFailures)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.prev, tt.outcome, tt.cfg)
			tt.check(t, got)
		})
	}
}

func TestAvailabilityConfigDefaults(t *testing.T) {
	got := AvailabilityConfig{}.withDefaults()
	require.Equal(t, 3, got.BrokenThreshold)
	require.Equal(t, 24*time.Hour, got.BrokenWindow)
	require.Equal(t, 7*24*time.Hour, got.DefaultInterval)
	require.Equal(t, 7*24*time.Hour, got.MaxRetryAfter)
}

func TestAvailabilityMarkRequiresParameters(t *testing.T) {
	// S14 主链路步 1：需参数且无实例 → requires_parameters，不发请求、
	// 无下次检查计划，失败痕迹一并清空。
	prev := AvailabilityState{
		Status: AvailabilityStatusOK, ConsecutiveFailures: 1,
		FirstFailureAt: ptrAt(availBase.Add(-time.Hour)),
		LastErrorCode:  OutcomeNetworkError,
	}
	got := MarkRequiresParameters(prev, availBase)
	require.Equal(t, AvailabilityStatusRequiresParams, got.Status)
	require.Equal(t, 0, got.ConsecutiveFailures)
	require.Nil(t, got.FirstFailureAt)
	require.Empty(t, got.LastErrorCode)
	require.Nil(t, got.NextCheckAt)
	require.NotNil(t, got.LastAttemptAt)
}
