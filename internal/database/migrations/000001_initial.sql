CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]*$'),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE source_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_type text NOT NULL CHECK (source_type IN ('task', 'note', 'briefing', 'repository', 'conversation')),
    external_key text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_complete_sync_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, source_type, external_key),
    UNIQUE (project_id, id)
);

CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_instance_id uuid NOT NULL,
    source_type text NOT NULL CHECK (source_type IN ('task', 'note', 'briefing', 'repository', 'conversation')),
    source_identity text NOT NULL,
    title text NOT NULL,
    current_revision_id uuid,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, source_instance_id) REFERENCES source_instances(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, source_type, source_instance_id, source_identity),
    UNIQUE (project_id, id),
    UNIQUE (source_instance_id, id)
);

CREATE TABLE revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id uuid NOT NULL,
    content_hash char(64) NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    normalized_text text NOT NULL,
    rendered_content text NOT NULL,
    renderer text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    UNIQUE (document_id, content_hash),
    UNIQUE (project_id, id),
    UNIQUE (document_id, id)
);

ALTER TABLE documents
    ADD CONSTRAINT documents_current_revision_fk
    FOREIGN KEY (id, current_revision_id)
    REFERENCES revisions(document_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE chunks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    revision_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    normalized_text text NOT NULL,
    structural_location jsonb NOT NULL DEFAULT '{}'::jsonb,
    token_count integer CHECK (token_count IS NULL OR token_count >= 0),
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english', normalized_text)) STORED,
    FOREIGN KEY (project_id, revision_id) REFERENCES revisions(project_id, id) ON DELETE CASCADE,
    UNIQUE (revision_id, ordinal),
    UNIQUE (project_id, id)
);

CREATE INDEX chunks_search_idx ON chunks USING gin (search_vector);
CREATE INDEX chunks_project_revision_idx ON chunks (project_id, revision_id);

CREATE TABLE embeddings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    chunk_id uuid NOT NULL,
    model text NOT NULL DEFAULT 'voyage/voyage-4' CHECK (model = 'voyage/voyage-4'),
    dimensions integer NOT NULL DEFAULT 1024 CHECK (dimensions = 1024),
    embedding vector(1024) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, chunk_id) REFERENCES chunks(project_id, id) ON DELETE CASCADE,
    UNIQUE (chunk_id, model)
);

CREATE INDEX embeddings_project_idx ON embeddings (project_id);
CREATE INDEX embeddings_cosine_idx ON embeddings USING hnsw (embedding vector_cosine_ops);

CREATE TABLE relationships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_document_id uuid NOT NULL,
    target_document_id uuid NOT NULL,
    relationship_type text NOT NULL CHECK (relationship_type IN ('task-depends-on')),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, source_document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, target_document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    CHECK (source_document_id <> target_document_id),
    UNIQUE (project_id, source_document_id, target_document_id, relationship_type)
);

CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE document_tags (
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id uuid NOT NULL,
    tag_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, tag_id) REFERENCES tags(project_id, id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, tag_id)
);

CREATE TYPE annotation_status AS ENUM ('open', 'resolved', 'dismissed');
CREATE SEQUENCE annotation_change_sequence;

CREATE TABLE annotations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id uuid NOT NULL,
    revision_id uuid,
    body text NOT NULL,
    status annotation_status NOT NULL DEFAULT 'open',
    attributed_username text NOT NULL,
    originating_operation text NOT NULL,
    selector jsonb NOT NULL,
    selected_quote text,
    quote_prefix text,
    quote_suffix text,
    structural_location jsonb NOT NULL DEFAULT '{}'::jsonb,
    original_content_hash char(64) NOT NULL CHECK (original_content_hash ~ '^[0-9a-f]{64}$'),
    source_provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
    copied_from_annotation_id uuid REFERENCES annotations(id) ON DELETE SET NULL,
    prior_target jsonb,
    resolved_at timestamptz,
    resolved_by text,
    tombstoned_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    change_sequence bigint NOT NULL DEFAULT nextval('annotation_change_sequence'),
    FOREIGN KEY (project_id, document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (document_id, revision_id) REFERENCES revisions(document_id, id) ON DELETE RESTRICT,
    CHECK ((tombstoned_at IS NULL AND revision_id IS NOT NULL) OR tombstoned_at IS NOT NULL),
    CHECK ((status = 'open' AND resolved_at IS NULL) OR status <> 'open')
);

CREATE INDEX annotations_project_status_idx ON annotations (project_id, status);
CREATE INDEX annotations_project_cursor_idx ON annotations (project_id, change_sequence);
CREATE INDEX documents_project_current_idx ON documents (project_id, source_type) WHERE deleted_at IS NULL;

CREATE FUNCTION touch_annotation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    NEW.change_sequence = nextval('annotation_change_sequence');
    RETURN NEW;
END;
$$;

CREATE TRIGGER annotations_touch_before_update
BEFORE UPDATE ON annotations
FOR EACH ROW EXECUTE FUNCTION touch_annotation();
