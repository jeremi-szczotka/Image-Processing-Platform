-- Migration 002: jobs table
-- Stores processing job metadata and tracks lifecycle status.

CREATE TYPE job_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED');

CREATE TABLE IF NOT EXISTS jobs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    image_id            UUID        NOT NULL REFERENCES images(id),
    operation           TEXT        NOT NULL,
    parameters          JSONB       NOT NULL DEFAULT '{}',
    status              job_status  NOT NULL DEFAULT 'PENDING',
    result_object_key   TEXT,
    result_content_type TEXT,
    result_file_size    BIGINT,
    attempt_count       INT         NOT NULL DEFAULT 0,
    error_message       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Outbox table for reliable RabbitMQ publishing (transactional outbox pattern)
CREATE TABLE IF NOT EXISTS outbox_events (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id     UUID        NOT NULL REFERENCES jobs(id),
    payload    JSONB       NOT NULL,
    published  BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX idx_jobs_status     ON jobs(status);
CREATE INDEX idx_jobs_image_id   ON jobs(image_id);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);
CREATE INDEX idx_outbox_unpublished ON outbox_events(created_at) WHERE published = false;

-- Auto-update updated_at on every change
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_jobs_updated_at
BEFORE UPDATE ON jobs
FOR EACH ROW EXECUTE FUNCTION update_updated_at();
