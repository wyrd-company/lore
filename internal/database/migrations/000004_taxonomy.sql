ALTER TABLE relationships
    DROP CONSTRAINT relationships_relationship_type_check,
    ADD CONSTRAINT relationships_relationship_type_check
        CHECK (relationship_type IN ('task-depends-on', 'note-related-to'));

CREATE TABLE terms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE document_terms (
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id uuid NOT NULL,
    term_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, term_id) REFERENCES terms(project_id, id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, term_id)
);

CREATE TABLE term_definitions (
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    term_id uuid NOT NULL,
    document_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, term_id) REFERENCES terms(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, document_id) REFERENCES documents(project_id, id) ON DELETE CASCADE,
    PRIMARY KEY (term_id, document_id)
);

CREATE INDEX document_terms_project_term_idx ON document_terms (project_id, term_id);
CREATE INDEX term_definitions_project_document_idx ON term_definitions (project_id, document_id);
