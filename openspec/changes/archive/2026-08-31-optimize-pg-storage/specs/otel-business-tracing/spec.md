# otel-business-tracing Delta — optimize-pg-storage

## ADDED Requirements

### Requirement: Tracing sample ratio defaults low and is configurable

The tracing subsystem SHALL default its span sample ratio to 0.05 (5%) instead of 1.0 (full sampling). Full sampling at ~820k spans/day keeps `otel_spans` at a steady 4.3GB under the 7-day retention and generates constant dead-tuple churn; 5% covers routine debugging while cutting daily span volume ~95%.

The sample ratio SHALL be overridable by environment variable (`TRACE_SAMPLE_RATIO`, accepted range 0.0–1.0) without code changes, so an incident session can temporarily restore a higher ratio. Invalid values (unparseable, out of range) SHALL fall back to the default 0.05 with a warn log.

Sampling SHALL use parent-based semantics: spans that inherit an already-sampled trace context are always recorded, so a sampled root request keeps its full downstream tree (including GORM/HTTP child spans within it).

#### Scenario: Default sampling is low

- **WHEN** the backend starts with no tracing-related environment variables
- **THEN** the effective sample ratio is 0.05 and otel_spans daily insert volume drops by roughly 95% versus full sampling

#### Scenario: Environment override raises sampling

- **WHEN** the backend starts with `TRACE_SAMPLE_RATIO=1.0`
- **THEN** the effective sample ratio is 1.0 (full sampling) without any code change

#### Scenario: Invalid override falls back safely

- **WHEN** the backend starts with `TRACE_SAMPLE_RATIO=abc` or `=2.5`
- **THEN** the backend starts successfully with the default 0.05 ratio and logs a warn about the invalid value

#### Scenario: Sampled root keeps its span tree

- **WHEN** a root span is selected by the sampler and it triggers child spans (LLM call, SQL query)
- **THEN** all descendant spans of the sampled root are recorded and linked in one trace, regardless of the 0.05 ratio
