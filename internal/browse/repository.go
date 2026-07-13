package browse

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

type ProjectSummary struct {
	ID               uuid.UUID      `json:"id"`
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	DocumentCount    int            `json:"documentCount"`
	SourceCount      int            `json:"sourceCount"`
	SourceTypeCounts map[string]int `json:"sourceTypeCounts"`
}

type SourceSummary struct {
	ID                 uuid.UUID       `json:"id"`
	SourceType         string          `json:"sourceType"`
	SourceInstance     string          `json:"sourceInstance"`
	Metadata           json.RawMessage `json:"metadata"`
	DocumentCount      int             `json:"documentCount"`
	LastCompleteSyncAt *time.Time      `json:"lastCompleteSyncAt,omitempty"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type DocumentSummary struct {
	ID                 uuid.UUID       `json:"id"`
	SourceType         string          `json:"sourceType"`
	SourceInstance     string          `json:"sourceInstance"`
	SourceIdentity     string          `json:"sourceIdentity"`
	Title              string          `json:"title"`
	RevisionID         uuid.UUID       `json:"revisionId"`
	Metadata           json.RawMessage `json:"metadata"`
	Tags               []string        `json:"tags"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
	ChunkCount         int             `json:"chunkCount"`
	EmbeddedChunkCount int             `json:"embeddedChunkCount"`
}

type RepositoryGroup struct {
	Repository string            `json:"repository"`
	Branch     string            `json:"branch"`
	Documents  []DocumentSummary `json:"documents"`
}

type BrowseResponse struct {
	Project       ProjectSummary    `json:"project"`
	Sources       []SourceSummary   `json:"sources"`
	Tags          []string          `json:"tags"`
	Tasks         []DocumentSummary `json:"tasks"`
	Notes         []DocumentSummary `json:"notes"`
	Briefings     []DocumentSummary `json:"briefings"`
	Repositories  []RepositoryGroup `json:"repositories"`
	Conversations []DocumentSummary `json:"conversations"`
}

type Relationship struct {
	Direction      string          `json:"direction"`
	Type           string          `json:"type"`
	DocumentID     uuid.UUID       `json:"documentId"`
	SourceIdentity string          `json:"sourceIdentity"`
	Title          string          `json:"title"`
	Metadata       json.RawMessage `json:"metadata"`
}

type RevisionSummary struct {
	ID              uuid.UUID `json:"id"`
	ContentHash     string    `json:"contentHash"`
	Renderer        string    `json:"renderer"`
	CreatedAt       time.Time `json:"createdAt"`
	Current         bool      `json:"current"`
	ChunkCount      int       `json:"chunkCount"`
	EmbeddedChunks  int       `json:"embeddedChunks"`
	AnnotationCount int       `json:"annotationCount"`
}

type DocumentDetail struct {
	DocumentSummary
	ContentHash     string            `json:"contentHash"`
	NormalizedText  string            `json:"normalizedText"`
	RenderedContent string            `json:"renderedContent"`
	Renderer        string            `json:"renderer"`
	Provenance      json.RawMessage   `json:"provenance"`
	Relationships   []Relationship    `json:"relationships"`
	Revisions       []RevisionSummary `json:"revisions"`
}

func (r *Repository) Projects(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.slug, p.name,
			count(DISTINCT d.id) FILTER (WHERE d.deleted_at IS NULL),
			count(DISTINCT s.id),
			coalesce(jsonb_object_agg(counts.source_type, counts.document_count)
				FILTER (WHERE counts.source_type IS NOT NULL), '{}'::jsonb)
		FROM projects p
		LEFT JOIN documents d ON d.project_id = p.id
		LEFT JOIN source_instances s ON s.project_id = p.id
		LEFT JOIN LATERAL (
			SELECT source_type, count(*) AS document_count
			FROM documents typed WHERE typed.project_id = p.id AND typed.deleted_at IS NULL
			GROUP BY source_type
		) counts ON true
		GROUP BY p.id, p.slug, p.name
		ORDER BY p.name, p.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []ProjectSummary
	for rows.Next() {
		var project ProjectSummary
		var countsJSON []byte
		if err := rows.Scan(&project.ID, &project.Slug, &project.Name, &project.DocumentCount, &project.SourceCount, &countsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(countsJSON, &project.SourceTypeCounts); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (r *Repository) Browse(ctx context.Context, projectID uuid.UUID) (BrowseResponse, error) {
	projects, err := r.project(ctx, projectID)
	if err != nil {
		return BrowseResponse{}, err
	}
	documents, err := r.documents(ctx, projectID)
	if err != nil {
		return BrowseResponse{}, err
	}
	sources, err := r.sources(ctx, projectID)
	if err != nil {
		return BrowseResponse{}, err
	}
	response := BrowseResponse{Project: projects, Sources: sources}
	tagSet := make(map[string]struct{})
	type repositoryKey struct{ repository, branch string }
	repositories := make(map[repositoryKey][]DocumentSummary)
	for _, document := range documents {
		for _, tag := range document.Tags {
			tagSet[tag] = struct{}{}
		}
		switch document.SourceType {
		case "task":
			response.Tasks = append(response.Tasks, document)
		case "note":
			response.Notes = append(response.Notes, document)
		case "briefing":
			response.Briefings = append(response.Briefings, document)
		case "conversation":
			response.Conversations = append(response.Conversations, document)
		case "repository":
			var metadata struct {
				Repository string `json:"repository"`
				Branch     string `json:"branch"`
			}
			_ = json.Unmarshal(document.Metadata, &metadata)
			repositories[repositoryKey{metadata.Repository, metadata.Branch}] = append(repositories[repositoryKey{metadata.Repository, metadata.Branch}], document)
		}
	}
	for tag := range tagSet {
		response.Tags = append(response.Tags, tag)
	}
	sort.Strings(response.Tags)
	for key, documents := range repositories {
		response.Repositories = append(response.Repositories, RepositoryGroup{Repository: key.repository, Branch: key.branch, Documents: documents})
	}
	sort.Slice(response.Repositories, func(i, j int) bool {
		if response.Repositories[i].Repository == response.Repositories[j].Repository {
			return response.Repositories[i].Branch < response.Repositories[j].Branch
		}
		return response.Repositories[i].Repository < response.Repositories[j].Repository
	})
	return response, nil
}

func (r *Repository) Document(ctx context.Context, projectID, documentID uuid.UUID) (DocumentDetail, error) {
	var detail DocumentDetail
	err := r.pool.QueryRow(ctx, `
		SELECT d.id, d.source_type, si.external_key, d.source_identity, d.title, r.id, r.metadata,
			ARRAY(SELECT t.name FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
				WHERE dt.document_id = d.id AND dt.project_id = d.project_id ORDER BY t.name),
			d.created_at, d.updated_at,
			(SELECT count(*) FROM chunks c WHERE c.revision_id = r.id),
			(SELECT count(*) FROM chunks c JOIN embeddings e ON e.chunk_id = c.id WHERE c.revision_id = r.id),
			r.content_hash, r.normalized_text, r.rendered_content, r.renderer, r.provenance
		FROM documents d
		JOIN source_instances si ON si.id = d.source_instance_id AND si.project_id = d.project_id
		JOIN revisions r ON r.id = d.current_revision_id AND r.project_id = d.project_id
		WHERE d.project_id = $1 AND d.id = $2 AND d.deleted_at IS NULL`, projectID, documentID).
		Scan(&detail.ID, &detail.SourceType, &detail.SourceInstance, &detail.SourceIdentity, &detail.Title,
			&detail.RevisionID, &detail.Metadata, &detail.Tags, &detail.CreatedAt, &detail.UpdatedAt,
			&detail.ChunkCount, &detail.EmbeddedChunkCount, &detail.ContentHash, &detail.NormalizedText,
			&detail.RenderedContent, &detail.Renderer, &detail.Provenance)
	if err != nil {
		return detail, err
	}
	detail.Relationships, err = r.relationships(ctx, projectID, documentID)
	if err != nil {
		return detail, err
	}
	detail.Revisions, err = r.Revisions(ctx, projectID, documentID)
	return detail, err
}

func (r *Repository) Revisions(ctx context.Context, projectID, documentID uuid.UUID) ([]RevisionSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.content_hash, r.renderer, r.created_at, r.id = d.current_revision_id,
			(SELECT count(*) FROM chunks c WHERE c.revision_id = r.id),
			(SELECT count(*) FROM chunks c JOIN embeddings e ON e.chunk_id = c.id WHERE c.revision_id = r.id),
			(SELECT count(*) FROM annotations a WHERE a.revision_id = r.id)
		FROM documents d
		JOIN revisions r ON r.document_id = d.id AND r.project_id = d.project_id
		WHERE d.project_id = $1 AND d.id = $2
		ORDER BY r.created_at DESC, r.id`, projectID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RevisionSummary
	for rows.Next() {
		var revision RevisionSummary
		if err := rows.Scan(&revision.ID, &revision.ContentHash, &revision.Renderer, &revision.CreatedAt,
			&revision.Current, &revision.ChunkCount, &revision.EmbeddedChunks, &revision.AnnotationCount); err != nil {
			return nil, err
		}
		result = append(result, revision)
	}
	return result, rows.Err()
}

func (r *Repository) project(ctx context.Context, projectID uuid.UUID) (ProjectSummary, error) {
	projects, err := r.Projects(ctx)
	if err != nil {
		return ProjectSummary{}, err
	}
	for _, project := range projects {
		if project.ID == projectID {
			return project, nil
		}
	}
	return ProjectSummary{}, pgx.ErrNoRows
}

func (r *Repository) documents(ctx context.Context, projectID uuid.UUID) ([]DocumentSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.source_type, si.external_key, d.source_identity, d.title, r.id, r.metadata,
			ARRAY(SELECT t.name FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
				WHERE dt.document_id = d.id AND dt.project_id = d.project_id ORDER BY t.name),
			d.created_at, d.updated_at,
			(SELECT count(*) FROM chunks c WHERE c.revision_id = r.id),
			(SELECT count(*) FROM chunks c JOIN embeddings e ON e.chunk_id = c.id WHERE c.revision_id = r.id)
		FROM documents d
		JOIN source_instances si ON si.id = d.source_instance_id AND si.project_id = d.project_id
		JOIN revisions r ON r.id = d.current_revision_id AND r.project_id = d.project_id
		WHERE d.project_id = $1 AND d.deleted_at IS NULL
		ORDER BY d.source_type, d.title, d.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DocumentSummary
	for rows.Next() {
		var document DocumentSummary
		if err := rows.Scan(&document.ID, &document.SourceType, &document.SourceInstance, &document.SourceIdentity,
			&document.Title, &document.RevisionID, &document.Metadata, &document.Tags, &document.CreatedAt,
			&document.UpdatedAt, &document.ChunkCount, &document.EmbeddedChunkCount); err != nil {
			return nil, err
		}
		result = append(result, document)
	}
	return result, rows.Err()
}

func (r *Repository) sources(ctx context.Context, projectID uuid.UUID) ([]SourceSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.source_type, s.external_key, s.metadata,
			count(d.id) FILTER (WHERE d.deleted_at IS NULL), s.last_complete_sync_at, s.updated_at
		FROM source_instances s
		LEFT JOIN documents d ON d.source_instance_id = s.id AND d.project_id = s.project_id
		WHERE s.project_id = $1
		GROUP BY s.id
		ORDER BY s.source_type, s.external_key`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SourceSummary
	for rows.Next() {
		var source SourceSummary
		if err := rows.Scan(&source.ID, &source.SourceType, &source.SourceInstance, &source.Metadata,
			&source.DocumentCount, &source.LastCompleteSyncAt, &source.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, source)
	}
	return result, rows.Err()
}

func (r *Repository) relationships(ctx context.Context, projectID, documentID uuid.UUID) ([]Relationship, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT 'dependency', rel.relationship_type, target.id, target.source_identity, target.title, rel.metadata
		FROM relationships rel
		JOIN documents target ON target.id = rel.target_document_id AND target.project_id = rel.project_id
		WHERE rel.project_id = $1 AND rel.source_document_id = $2 AND target.deleted_at IS NULL
		UNION ALL
		SELECT 'dependent', rel.relationship_type, source.id, source.source_identity, source.title, rel.metadata
		FROM relationships rel
		JOIN documents source ON source.id = rel.source_document_id AND source.project_id = rel.project_id
		WHERE rel.project_id = $1 AND rel.target_document_id = $2 AND source.deleted_at IS NULL
		ORDER BY 1, 5`, projectID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Relationship
	for rows.Next() {
		var relationship Relationship
		if err := rows.Scan(&relationship.Direction, &relationship.Type, &relationship.DocumentID,
			&relationship.SourceIdentity, &relationship.Title, &relationship.Metadata); err != nil {
			return nil, err
		}
		result = append(result, relationship)
	}
	return result, rows.Err()
}

func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
