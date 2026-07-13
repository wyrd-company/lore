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

CREATE INDEX revisions_repository_idx ON revisions ((metadata ->> 'repository'));
CREATE INDEX revisions_branch_idx ON revisions ((metadata ->> 'branch'));
CREATE INDEX revisions_created_idx ON revisions (project_id, created_at DESC);
