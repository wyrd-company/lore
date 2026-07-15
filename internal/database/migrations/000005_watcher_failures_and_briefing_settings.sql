-- ---
-- relationships:
--   implements: system
-- ---

CREATE TABLE ingestion_failures (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_instance_id uuid NOT NULL,
    path text NOT NULL CHECK (btrim(path) <> ''),
    message text NOT NULL CHECK (btrim(message) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, source_instance_id)
        REFERENCES source_instances(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, source_instance_id, path),
    UNIQUE (project_id, id)
);

CREATE INDEX ingestion_failures_project_updated_idx
    ON ingestion_failures (project_id, updated_at DESC, id);

CREATE TABLE briefing_settings (
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id uuid NOT NULL,
    category text CHECK (category IS NULL OR btrim(category) <> ''),
    is_home boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, document_id)
        REFERENCES documents(project_id, id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, document_id)
);

CREATE UNIQUE INDEX briefing_settings_one_home_per_project_idx
    ON briefing_settings (project_id) WHERE is_home;
