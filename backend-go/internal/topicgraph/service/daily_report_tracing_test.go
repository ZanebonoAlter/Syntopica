package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// testExporter captures all spans created during tests. It is backed by an
// in-memory exporter wired to the global TracerProvider in TestMain, so the
// production code path tracing.Tracer(tracing.ServiceName) -> otel.Tracer(...)
// emits real, inspectable spans without touching PostgreSQL or the real DB
// exporter. This mirrors the existing pure-unit-test style of this package
// (no testcontainer, no airouter mock) — only functions whose early-return
// paths skip the real LLM are exercised.
var testExporter *tracetest.InMemoryExporter

func TestMain(m *testing.M) {
	testExporter = tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(testExporter)),
	)
	// Set before any test (and before any production code calls otel.Tracer),
	// so tracing.Tracer(tracing.ServiceName) resolves to the test provider.
	otel.SetTracerProvider(tp)
	m.Run()
}

// workflowSpanPrefix is the shared prefix for every daily-report orchestration
// span added by otel-tracing-completion task 2.
const workflowSpanPrefix = "workflow.daily_report."

// TestDailyReportStepSpans_NamedAndParentedUnderGenerate asserts that each
// daily-report step function opens a span named workflow.daily_report.<step>
// and that the span is a direct child of the generate root (non-orphan). Each
// function is driven through an early-return input so no real LLM/embed call
// is made; the span is still created because tracer.Start runs before the
// early-return guard.
func TestDailyReportStepSpans_NamedAndParentedUnderGenerate(t *testing.T) {
	testExporter.Reset()

	ctx := context.Background()
	// Root span — mirrors what GenerateDailyReport now opens at its entry.
	ctx, rootSpan := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.generate")
	rootID := rootSpan.SpanContext().SpanID()

	// dedup — empty input early-returns, span still recorded.
	DeduplicateTags(ctx, nil, nil)

	// cluster_tags — <=2 tags skips the LLM, span still recorded.
	_, _ = ClusterTags(ctx, []repository.TagInput{{ID: 1, Label: "solo"}}, nil, nil)

	// highlights — empty tags early-returns, span still recorded.
	_, _ = GenerateHighlights(ctx, nil, nil)

	// cluster_threads — cluster references no real tag → clusterTags empty →
	// early-returns before airouter.NewRouter().Chat, span still recorded.
	_, _, _ = GenerateClusterThreads(ctx,
		repository.ClusterGroup{GroupName: "g", TagIDs: []uint{999}},
		[]repository.TagInput{{ID: 1, Label: "x"}})

	// thread_fit — no sections with embeddings → early-returns before calling
	// the injected embed func (nil is safe: never invoked).
	computeThreadFitDistances(ctx, nil, nil, 1, nil)

	rootSpan.End()

	want := []string{
		"workflow.daily_report.dedup",
		"workflow.daily_report.cluster_tags",
		"workflow.daily_report.highlights",
		"workflow.daily_report.cluster_threads",
		"workflow.daily_report.thread_fit",
	}
	spans := testExporter.GetSpans()
	gotNames := make(map[string]bool, len(spans))
	for _, s := range spans {
		gotNames[s.Name] = true
	}
	for _, n := range want {
		assert.True(t, gotNames[n], "expected span %q in captured timeline", n)
	}

	// Every workflow.daily_report.* step span (except the generate root itself)
	// MUST be parented directly under the generate root — never orphaned and
	// never dangling off the wrong parent.
	for _, s := range spans {
		if s.Name == "workflow.daily_report.generate" {
			continue
		}
		if !strings.HasPrefix(s.Name, workflowSpanPrefix) {
			continue
		}
		assert.True(t, s.Parent.IsValid(),
			"span %q must have a parent (not orphan)", s.Name)
		assert.Equal(t, rootID, s.Parent.SpanID(),
			"span %q must be a direct child of the generate root", s.Name)
	}
}

// TestDailyReportSpans_ConcurrentClusterThreadsShareParent is the Step5
// concurrency guard (tasks.md §2.3 / design.md §2.3). The orchestrator fans
// out K GenerateClusterThreads calls across goroutines; every resulting
// cluster_threads span MUST attach to the same generate parent captured
// BEFORE the goroutine launch — not one-parent-per-goroutine and not orphaned.
//
// This test replicates the orchestrator's pattern: the parent span context
// (ctx carrying the generate root) is taken outside the goroutine and passed
// in via the closure. GenerateClusterThreads derives its span from that ctx,
// so all K concurrent spans share the root as parent.
func TestDailyReportSpans_ConcurrentClusterThreadsShareParent(t *testing.T) {
	testExporter.Reset()

	ctx := context.Background()
	// Parent captured OUTSIDE the goroutines — the exact rule the design
	// imposes on the orchestrator's Step5.
	ctx, rootSpan := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.generate")
	rootID := rootSpan.SpanContext().SpanID()

	tags := []repository.TagInput{{ID: 1, Label: "x"}}
	clusters := []repository.ClusterGroup{
		{GroupName: "c1", TagIDs: []uint{999}}, // no matching tag → early return, no LLM
		{GroupName: "c2", TagIDs: []uint{998}},
		{GroupName: "c3", TagIDs: []uint{997}},
		{GroupName: "c4", TagIDs: []uint{996}},
	}

	var wg sync.WaitGroup
	for _, cluster := range clusters {
		wg.Add(1)
		go func(c repository.ClusterGroup) {
			defer wg.Done()
			_, _, _ = GenerateClusterThreads(ctx, c, tags)
		}(cluster)
	}
	wg.Wait()
	rootSpan.End()

	spans := testExporter.GetSpans()
	var threadSpans []tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "workflow.daily_report.cluster_threads" {
			threadSpans = append(threadSpans, s)
		}
	}
	require.Lenf(t, threadSpans, len(clusters),
		"expected one cluster_threads span per goroutine (got %d)", len(threadSpans))

	for _, s := range threadSpans {
		assert.True(t, s.Parent.IsValid(),
			"concurrent cluster_threads span must have a parent (not orphan)")
		assert.Equal(t, rootID, s.Parent.SpanID(),
			"all concurrent cluster_threads spans share the generate root parent")
	}
}
