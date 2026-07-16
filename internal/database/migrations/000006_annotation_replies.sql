ALTER TABLE annotations ADD CONSTRAINT annotations_project_id_id_unique UNIQUE (project_id, id);

CREATE TABLE annotation_replies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    annotation_id uuid NOT NULL REFERENCES annotations(id) ON DELETE CASCADE,
    body text NOT NULL CHECK (length(btrim(body)) > 0),
    attributed_username text NOT NULL CHECK (length(btrim(attributed_username)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, annotation_id) REFERENCES annotations(project_id, id) ON DELETE CASCADE
);

CREATE INDEX annotation_replies_annotation_idx ON annotation_replies (project_id, annotation_id, created_at, id);

ALTER TABLE annotation_events DROP CONSTRAINT annotation_events_operation_check;
ALTER TABLE annotation_events ADD CONSTRAINT annotation_events_operation_check
    CHECK (operation IN ('create', 'update', 'resolve', 'dismiss', 'reopen', 'copy', 'move', 'cleanup', 'reply'));
