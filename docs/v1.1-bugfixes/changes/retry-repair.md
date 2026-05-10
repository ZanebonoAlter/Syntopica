# Retry Repair Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix retry gaps in article completion and summary generation, and make AI call logs identify concrete retry targets.

**Architecture:** Keep the current schedulers and route-based AI calls, but repair the state transitions so transient failures remain retryable until the configured retry limit. Add deterministic summary batch dedupe keyed by feed plus article set, and enrich AI log metadata with stable business identifiers.

**Tech Stack:** Go, GORM, SQLite, Gin, existing AI router and scheduler jobs.

---

### Task 1: Lock article completion retry behavior with tests

**Files:**
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service_test.go`

**Steps:**
1. Add a failing test for first failure staying retryable.
2. Add assertions for second failure reaching terminal `failed` when max retries is hit.
3. Add assertions that `ai_call_logs.request_meta` includes `article_id` and `feed_id`.

### Task 2: Lock auto summary batch dedupe with tests

**Files:**
- Modify: `backend-go/internal/jobs/auto_summary_test.go`

**Steps:**
1. Add a failing test where batch 1 already has a saved summary.
2. Assert the scheduler skips the existing batch and only calls AI for the remaining batch.
3. Assert summary call metadata includes `feed_id`, `batch_num`, and `article_ids`.

### Task 3: Lock manual summary queue dedupe with tests

**Files:**
- Modify: `backend-go/internal/domain/summaries/summary_queue_test.go`

**Steps:**
1. Add a failing test where queue batch 1 already exists.
2. Assert the queue only generates the missing batch.
3. Assert summary call metadata includes `feed_id`, `batch_num`, and `article_ids`.

### Task 4: Implement retry state fixes and metadata enrichment

**Files:**
- Modify: `backend-go/internal/domain/contentprocessing/content_completion_service.go`
- Modify: `backend-go/internal/jobs/auto_summary.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue.go`
- Modify: `backend-go/internal/domain/topictypes/types.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`
- Modify: `backend-go/internal/domain/topicextraction/article_tagger.go`
- Modify: `backend-go/internal/domain/topicextraction/extractor_enhanced.go`

**Steps:**
1. Keep article completion failures retryable until the configured retry limit is reached.
2. Add shared summary request metadata builders and existing-batch lookup helpers.
3. Skip already-saved summary batches instead of regenerating them.
4. Pass `article_id` / `summary_id` through topic tagging metadata.

### Task 5: Verify with targeted tests

**Files:**
- Verify only

**Steps:**
1. Run targeted package tests for content completion.
2. Run targeted package tests for jobs auto summary.
3. Run targeted package tests for summary queue and topic extraction if touched.
