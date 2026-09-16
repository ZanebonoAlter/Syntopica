#!/bin/sh
# Entrypoint for the public read-only demo container.
#
# Flow:
#   1. Start the backend in the background. On first boot it runs AutoMigrate +
#      versioned migrations to create the full schema (tables, indexes,
#      triggers, pgvector columns).
#   2. Wait for /health to return 200, which means the schema is ready.
#   3. Import the sanitized seed data with psql.
#   4. Bring the backend to the foreground (wait) so the container stays up.
#
# The seed import is idempotent only against a FRESH database (the demo
# postgres container does not mount a data volume, so every `docker compose up`
# starts clean). Never run this against a populated production database.
set -e

echo "[demo] starting backend (schema bootstrap)..."
/app/syntopica &
BACKEND_PID=$!

echo "[demo] waiting for backend health..."
until curl -sf http://localhost:5000/health >/dev/null 2>&1; do
    sleep 1
done
echo "[demo] backend healthy, schema ready."

echo "[demo] clearing bootstrap/default data before seed import..."
psql -v ON_ERROR_STOP=1 <<'SQL'
TRUNCATE TABLE
  daily_report_section_relations,
  daily_report_threads,
  daily_report_sections,
  board_daily_reports,
  narrative_summaries,
  article_topic_tags,
  topic_tag_relations,
  ai_route_providers,
  board_composition,
  topic_tag_board_labels,
  topic_tag_semantic_labels,
  articles,
  feeds,
  topic_tags,
  narrative_boards,
  scheduler_tasks,
  embedding_config,
  ai_settings,
  ai_routes,
  ai_providers,
  semantic_labels,
  categories,
  reading_behaviors,
  user_preferences
RESTART IDENTITY CASCADE;
SQL

echo "[demo] importing sanitized seed data..."
psql -v ON_ERROR_STOP=1 -f /app/seed.sql
echo "[demo] seed import complete."

echo "[demo] demo ready at http://localhost:5000"
wait "$BACKEND_PID"
