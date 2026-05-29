package tracing

type Config struct {
	Enabled       bool
	TableName     string
	RetentionDays int
	BufferSize    int
	FlushInterval int
	Debug         bool
}

func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		TableName:     "otel_spans",
		RetentionDays: 7,
		BufferSize:    100,
		FlushInterval: 5,
	}
}
