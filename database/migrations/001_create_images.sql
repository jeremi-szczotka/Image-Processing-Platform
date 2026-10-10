-- Migration 001: images table
-- Stores metadata for uploaded original images.

CREATE TABLE IF NOT EXISTS images (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    original_filename TEXT        NOT NULL,
    object_key        TEXT        NOT NULL UNIQUE,
    content_type      TEXT        NOT NULL,
    file_size         BIGINT      NOT NULL,
    width             INT         NOT NULL,
    height            INT         NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
