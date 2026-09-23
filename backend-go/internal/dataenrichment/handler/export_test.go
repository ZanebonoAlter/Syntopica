package handler

import (
	"context"
	"time"
)

// StartHangingSignalReportJob parks a RUNNING board_signal_report job on the
// given candidate — test-only seam for the derived「研究中」status (design §3:
// the running bit is never persisted, it comes from live jobs, so tests must
// be able to park one). The job stays running until block is closed.
func (h *EnrichmentHandler) StartHangingSignalReportJob(boardID, candidateID uint, block <-chan struct{}) string {
	st, err := h.analysis.StartSignal(AnalysisScopeBoard, boardID, AnalysisJobKindBoardSignalReport, time.Minute,
		func(ctx context.Context, report func(SignalJobPatch)) error {
			report(SignalJobPatch{Phase: SignalPhaseResearch, CandidateID: candidateID})
			<-block
			return nil
		})
	if err != nil {
		panic("start hanging report job: " + err.Error())
	}
	// 等 candidateID patch 对派生状态查询可见（fn 在 goroutine 里打 patch，
	// helper 返回即意味着「研究中」已可被观察到，消除测试竞态）。
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := h.analysis.RunningSignalReportForCandidate(boardID, candidateID); ok {
			return st.JobID
		}
		if time.Now().After(deadline) {
			panic("hanging report job patch never became visible")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
