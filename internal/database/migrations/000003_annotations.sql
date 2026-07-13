ALTER TABLE annotations
    ADD COLUMN revision_identity uuid,
    ADD COLUMN updated_by text;

UPDATE annotations
SET revision_identity = revision_id,
    updated_by = attributed_username;

ALTER TABLE annotations
    ALTER COLUMN revision_identity SET NOT NULL,
    ALTER COLUMN updated_by SET NOT NULL,
    ADD CONSTRAINT annotations_resolution_consistent CHECK (
        (status = 'open' AND resolved_at IS NULL AND resolved_by IS NULL)
        OR
        (status <> 'open' AND resolved_at IS NOT NULL AND resolved_by IS NOT NULL)
    );

CREATE TABLE annotation_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    annotation_id uuid NOT NULL REFERENCES annotations(id) ON DELETE CASCADE,
    operation text NOT NULL CHECK (operation IN ('create', 'update', 'resolve', 'dismiss', 'reopen', 'copy', 'move', 'cleanup')),
    attributed_username text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX annotations_project_document_idx ON annotations (project_id, document_id, change_sequence);
CREATE INDEX annotations_revision_identity_idx ON annotations (project_id, revision_identity);
CREATE INDEX annotation_events_annotation_idx ON annotation_events (project_id, annotation_id, id);
