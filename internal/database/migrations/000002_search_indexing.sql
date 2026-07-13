DROP INDEX IF EXISTS chunks_search_idx;

ALTER TABLE chunks
    DROP COLUMN search_vector,
    ADD COLUMN content_kind text NOT NULL DEFAULT 'body'
        CHECK (content_kind IN ('body', 'user', 'assistant', 'thinking')),
    ADD COLUMN search_vector tsvector NOT NULL DEFAULT ''::tsvector;

CREATE INDEX chunks_search_idx ON chunks USING gin (search_vector);
CREATE INDEX chunks_project_kind_idx ON chunks (project_id, content_kind);

CREATE TABLE embedding_jobs (
    chunk_id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    model text NOT NULL DEFAULT 'voyage/voyage-4'
        CHECK (model = 'voyage/voyage-4'),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, chunk_id) REFERENCES chunks(project_id, id) ON DELETE CASCADE
);

CREATE INDEX embedding_jobs_available_idx
    ON embedding_jobs (available_at, locked_until, created_at);

UPDATE chunks c
SET search_vector =
    setweight(to_tsvector('english', coalesce(c.normalized_text, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(d.title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce((
        SELECT string_agg(t.name, ' ' ORDER BY t.name)
        FROM document_tags dt
        JOIN tags t ON t.id = dt.tag_id AND t.project_id = dt.project_id
        WHERE dt.document_id = d.id AND dt.project_id = d.project_id
    ), '')), CASE WHEN d.source_type IN ('task', 'note') THEN 'A'::"char" ELSE 'C'::"char" END) ||
    setweight(to_tsvector('english', coalesce((
        CASE WHEN d.source_type = 'conversation' THEN r.metadata - 'messages' ELSE r.metadata END
    )::text, '')), 'C')
FROM revisions r
JOIN documents d ON d.id = r.document_id AND d.project_id = r.project_id
WHERE c.revision_id = r.id AND c.project_id = r.project_id;

INSERT INTO embedding_jobs (chunk_id, project_id, model)
SELECT c.id, c.project_id, 'voyage/voyage-4'
FROM chunks c
LEFT JOIN embeddings e ON e.chunk_id = c.id AND e.model = 'voyage/voyage-4'
WHERE e.id IS NULL
ON CONFLICT (chunk_id) DO NOTHING;

CREATE INDEX revisions_repository_idx ON revisions ((metadata ->> 'repository'));
CREATE INDEX revisions_branch_idx ON revisions ((metadata ->> 'branch'));
CREATE INDEX revisions_created_idx ON revisions (project_id, created_at DESC);
