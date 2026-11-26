<!-- domain: dev -->

# configuration-docs

## Purpose

TBD: This spec governs the accuracy and maintenance of configuration documentation (`docs/reference/configuration.md`) relative to the actual codebase.

## Requirements

### Requirement: configuration.md SHALL only document environment variables that exist in code

The `configuration.md` document SHALL NOT document environment variables, helper functions, or source files that do not exist in the codebase.

#### Scenario: TOPIC_ANALYSIS section is removed
- **WHEN** a developer reads `configuration.md`
- **THEN** the document SHALL NOT contain a "Topic Analysis 调优" section listing `TOPIC_ANALYSIS_MAX_TOKENS`, `TOPIC_ANALYSIS_TEMPERATURE`, `TOPIC_ANALYSIS_TIMEOUT_SECONDS`, or `TOPIC_ANALYSIS_RETRY_COUNT` (none are read by any code; `grep -rn 'TOPIC_ANALYSIS_' backend-go/internal/` returns zero matches)
- **AND** SHALL NOT reference `parseEnvInt` / `parseEnvFloat` (deleted)
- **AND** SHALL NOT reference `internal/domain/topicanalysis/ai_analysis.go` (the `topicanalysis` package is deleted)

#### Scenario: REDIS_URL description matches current code usage
- **WHEN** a developer reads the `REDIS_URL` description
- **THEN** the description SHALL reflect the actual current use of Redis in the codebase (to be confirmed against source at implementation time), and SHALL NOT claim it backs a "Topic 分析任务队列" if that subsystem no longer exists
