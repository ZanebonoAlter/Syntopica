package tracing

type Config struct {
	Enabled       bool
	TableName     string
	RetentionDays int
	BufferSize    int
	FlushInterval int
	Debug         bool

	// SampleRatio controls head sampling of root spans via
	// ParentBased(TraceIDRatioBased(ratio)). 1.0 = sample all; <1.0 samples a
	// fraction of root spans; child spans of a sampled root are always kept.
	SampleRatio float64
	// InstrumentGORM toggles gormotel plugin mounting (DB operation spans).
	InstrumentGORM bool
	// InstrumentHTTP toggles otelhttp transport wrapping in httpclient factory.
	InstrumentHTTP bool
}

func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		TableName:      "otel_spans",
		RetentionDays:  7,
		BufferSize:     100,
		FlushInterval:  5,
		SampleRatio:    0.05,
		InstrumentGORM: true,
		InstrumentHTTP: true,
	}
}
