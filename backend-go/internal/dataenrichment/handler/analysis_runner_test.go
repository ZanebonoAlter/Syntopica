package handler

import (
	"context"
	"errors"
	"testing"
	"time"
)

// eventually polls cond until it returns true or the deadline passes —
// the anti-flake replacement for fixed time.Sleep waits (M9 runner tests).
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: condition not met within 2s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// M8.1 分析 ctx 脱离触发请求：父 ctx 取消后后台分析照常完成。
// （2026-08-27 生产实锤：同步 HTTP 下客户端离开页面 → "context canceled"。）
func TestAnalysisRunner_SurvivesParentCancel(t *testing.T) {
	r := newAnalysisRunner()
	parent, cancelParent := context.WithCancel(context.Background())
	_ = parent // the runner deliberately ignores any parent — only its cancel matters

	st, err := r.Start(AnalysisScopeBoard, 1, AnalysisJobKindBoardBrief, time.Minute, func(ctx context.Context) (uint, error) {
		// Simulate the client leaving: the triggering request's ctx dies
		// mid-analysis. The runner-supplied ctx must stay alive.
		cancelParent()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return 42, nil // survived — parent cancel did not propagate
		}
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st.JobID == "" || st.JobKind != AnalysisJobKindBoardBrief || !st.Running {
		t.Fatalf("start identity: %+v", st)
	}

	eventually(t, "job survives parent cancel", func() bool {
		byID, ok := r.StatusByJobID(st.JobID)
		return ok && byID.Finished
	})
	byID, _ := r.StatusByJobID(st.JobID)
	if byID.Error != "" || byID.ResultID != 42 || byID.JobKind != AnalysisJobKindBoardBrief {
		t.Fatalf("job result: %+v", byID)
	}
}

// M8.2 防重入：同目标在跑 → RunningJobError（携带当前任务身份）；
// 完成后可再次触发；不同目标不受影响。
func TestAnalysisRunner_RejectsDuplicateWhileRunning(t *testing.T) {
	r := newAnalysisRunner()
	block := make(chan struct{})
	done := make(chan struct{})

	st1, err := r.Start(AnalysisScopeTopic, 7, AnalysisJobKindTopic, time.Minute, func(ctx context.Context) (uint, error) {
		<-block
		close(done)
		return 9, nil
	})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	_, err = r.Start(AnalysisScopeTopic, 7, AnalysisJobKindTopic, time.Minute, func(ctx context.Context) (uint, error) {
		return 0, errors.New("must not run")
	})
	var runErr *RunningJobError
	if !errors.As(err, &runErr) {
		t.Fatalf("duplicate start: want RunningJobError, got %v", err)
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatal("sentinel errors.Is(ErrAlreadyRunning) must keep working")
	}
	// 409 载荷 = 当前在跑任务的完整身份，前端据此恢复轮询而非新开任务。
	if runErr.Current.JobID != st1.JobID || runErr.Current.JobKind != AnalysisJobKindTopic ||
		runErr.Current.Scope != AnalysisScopeTopic || runErr.Current.TargetID != 7 || !runErr.Current.Running {
		t.Fatalf("409 identity: started=%+v conflict=%+v", st1, runErr.Current)
	}
	// Different target is unaffected.
	if _, err := r.Start(AnalysisScopeTopic, 8, AnalysisJobKindTopic, time.Minute, func(ctx context.Context) (uint, error) {
		return 1, nil
	}); err != nil {
		t.Fatalf("other target start: %v", err)
	}

	close(block)
	<-done
	// After finish, re-trigger is allowed (with a fresh job id).
	st2, err := r.Start(AnalysisScopeTopic, 7, AnalysisJobKindTopic, time.Minute, func(ctx context.Context) (uint, error) {
		return 10, nil
	})
	if err != nil {
		t.Fatalf("re-start after finish: %v", err)
	}
	if st2.JobID == st1.JobID {
		t.Fatal("sequential jobs on one target must get distinct job ids")
	}
}

// D9 同版块跨 kind 互斥：brief 在跑时 investigation 触发被 409 且携当前
// brief 身份（防止把 investigation 当 brief / 反之）；不同版块并行不受影响。
func TestAnalysisRunner_SameBoardCrossKindSerialized(t *testing.T) {
	r := newAnalysisRunner()
	block := make(chan struct{})
	release := func() { close(block) }
	defer func() { // safety release if the test fails early
		select {
		case <-block:
		default:
			release()
		}
	}()

	brief, err := r.Start(AnalysisScopeBoard, 5, AnalysisJobKindBoardBrief, time.Minute, func(ctx context.Context) (uint, error) {
		<-block
		return 100, nil
	})
	if err != nil {
		t.Fatalf("brief start: %v", err)
	}

	_, err = r.Start(AnalysisScopeBoard, 5, AnalysisJobKindBoardInvestigation, time.Minute, func(ctx context.Context) (uint, error) {
		return 0, errors.New("must not run")
	})
	var runErr *RunningJobError
	if !errors.As(err, &runErr) {
		t.Fatalf("cross-kind on same board: want RunningJobError, got %v", err)
	}
	if runErr.Current.JobID != brief.JobID || runErr.Current.JobKind != AnalysisJobKindBoardBrief {
		t.Fatalf("cross-kind 409 must carry the running brief identity, got %+v", runErr.Current)
	}

	// Different board runs in parallel while board 5's brief is blocked.
	inv, err := r.Start(AnalysisScopeBoard, 6, AnalysisJobKindBoardInvestigation, time.Minute, func(ctx context.Context) (uint, error) {
		return 200, nil
	})
	if err != nil {
		t.Fatalf("other board must run in parallel: %v", err)
	}
	eventually(t, "board 6 investigation finishes", func() bool {
		st, ok := r.StatusByJobID(inv.JobID)
		return ok && st.Finished
	})

	release()
	eventually(t, "board 5 brief finishes", func() bool {
		st, ok := r.StatusByJobID(brief.JobID)
		return ok && st.Finished
	})
}

// D9 job 身份与生命周期查询：每次 Start 返回唯一非空 job_id 与 kind；
// 完成/出错/超时/panic 均按 job_id 可查；按 scope/id 查询返回当前或最近
// 任务（重进恢复），被替换的旧 job 仍按其 job_id 可查。
func TestAnalysisRunner_JobIdentityAndRecovery(t *testing.T) {
	r := newAnalysisRunner()

	// Finished (ok) job.
	ok1, err := r.Start(AnalysisScopeBoard, 3, AnalysisJobKindBoardBrief, time.Minute, func(ctx context.Context) (uint, error) {
		return 11, nil
	})
	if err != nil {
		t.Fatalf("ok start: %v", err)
	}
	// Failed job.
	fail, err := r.Start(AnalysisScopeBoard, 4, AnalysisJobKindBoardInvestigation, time.Minute, func(ctx context.Context) (uint, error) {
		return 0, errors.New("kaput")
	})
	if err != nil {
		t.Fatalf("fail start: %v", err)
	}
	eventually(t, "ok job finishes", func() bool {
		st, ok := r.StatusByJobID(ok1.JobID)
		return ok && st.Finished
	})
	eventually(t, "failed job finishes", func() bool {
		st, ok := r.StatusByJobID(fail.JobID)
		return ok && st.Finished
	})
	st, _ := r.StatusByJobID(fail.JobID)
	if st.Error == "" || st.ResultID != 0 {
		t.Fatalf("failed job by id: %+v", st)
	}
	if ok1.JobID == fail.JobID {
		t.Fatal("distinct jobs must have distinct ids")
	}

	// By-target status reflects the newest job per target (re-entry recovery).
	stBoard4, found := r.Status(AnalysisScopeBoard, 4)
	if !found || stBoard4.JobID != fail.JobID || stBoard4.JobKind != AnalysisJobKindBoardInvestigation {
		t.Fatalf("by-target status must return the newest job: %+v found=%v", stBoard4, found)
	}

	// Replacing the target's job keeps the old one queryable by its own id.
	ok2, err := r.Start(AnalysisScopeBoard, 4, AnalysisJobKindBoardInvestigation, time.Minute, func(ctx context.Context) (uint, error) {
		return 12, nil
	})
	if err != nil {
		t.Fatalf("replace start: %v", err)
	}
	eventually(t, "replacement finishes", func() bool {
		st, ok := r.StatusByJobID(ok2.JobID)
		return ok && st.Finished
	})
	stBoard4b, _ := r.Status(AnalysisScopeBoard, 4)
	if stBoard4b.JobID != ok2.JobID {
		t.Fatalf("by-target status must return the newest job after replacement: %+v", stBoard4b)
	}
	stOld, okOld := r.StatusByJobID(fail.JobID)
	if !okOld || stOld.ResultID != 0 || stOld.Error == "" {
		t.Fatalf("old job must stay queryable by id: %+v ok=%v", stOld, okOld)
	}

	// Unknown id → not found (process restart equivalent).
	if _, ok := r.StatusByJobID("no-such-job"); ok {
		t.Fatal("unknown job_id must not resolve")
	}
}

// M8.3 panic 恢复：崩溃的分析变成 error 状态（按 job_id 可查），不拖垮进程。
func TestAnalysisRunner_PanicBecomesError(t *testing.T) {
	r := newAnalysisRunner()
	st, err := r.Start(AnalysisScopeBoard, 3, AnalysisJobKindBoardBrief, time.Minute, func(ctx context.Context) (uint, error) {
		panic("boom")
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	eventually(t, "panicked job finishes", func() bool {
		byID, ok := r.StatusByJobID(st.JobID)
		return ok && byID.Finished
	})
	byID, _ := r.StatusByJobID(st.JobID)
	if byID.Error == "" || !contains(byID.Error, "boom") {
		t.Fatalf("panic must surface as error, got %+v", byID)
	}
}

// M8.4 超时：fn 卡死时 ctx 到期，分析以 context deadline 错误收场。
func TestAnalysisRunner_TimeoutKillsRun(t *testing.T) {
	r := newAnalysisRunner()
	st, err := r.Start(AnalysisScopeBoard, 4, AnalysisJobKindBoardInvestigation, 30*time.Millisecond, func(ctx context.Context) (uint, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	eventually(t, "timed-out job finishes", func() bool {
		byID, ok := r.StatusByJobID(st.JobID)
		return ok && byID.Finished
	})
	byID, _ := r.StatusByJobID(st.JobID)
	if byID.Error == "" || byID.ResultID != 0 {
		t.Fatalf("timed-out job: %+v", byID)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
