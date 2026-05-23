package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"syntopica-backend/internal/platform/logging"
)

func TraceSchedulerTick(schedulerName, trigger string, fn func(ctx context.Context)) {
	ctx, span := Tracer("scheduler").Start(
		context.Background(),
		"scheduler."+schedulerName+".cycle",
		trace.WithNewRoot(),
		trace.WithAttributes(
			attribute.String("scheduler.name", schedulerName),
			attribute.String("scheduler.trigger", trigger),
		),
	)
	defer span.End()

	if !span.IsRecording() {
		logging.Infof("[tracing] scheduler %q tick: tracing not recording", schedulerName)
	}

	fn(ctx)
}

func TraceAsyncOp(parentCtx context.Context, opName string, fn func(ctx context.Context)) {
	GoWithTrace(parentCtx, "async."+opName, fn)
}
