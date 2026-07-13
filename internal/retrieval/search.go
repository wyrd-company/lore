package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const reciprocalRankConstant = 60.0

type Filters struct {
	SourceTypes  []string   `json:"sourceTypes"`
	Branches     []string   `json:"branches"`
	Repositories []string   `json:"repositories"`
	Tags         []string   `json:"tags"`
	CreatedFrom  *time.Time `json:"createdFrom,omitempty"`
	CreatedTo    *time.Time `json:"createdTo,omitempty"`
}

type Request struct {
	Query   string
	Filters Filters
	Limit   int
}

type Response struct {
	Query    string           `json:"query"`
	Filters  Filters          `json:"filters"`
	Modes    Modes            `json:"modes"`
	Warnings []string         `json:"warnings,omitempty"`
	Results  []DocumentResult `json:"results"`
}

type Modes struct {
	Keyword bool `json:"keyword"`
	Vector  bool `json:"vector"`
}

type DocumentResult struct {
	ID             uuid.UUID       `json:"id"`
	SourceType     string          `json:"sourceType"`
	SourceInstance string          `json:"sourceInstance"`
	SourceIdentity string          `json:"sourceIdentity"`
	Title          string          `json:"title"`
	RevisionID     uuid.UUID       `json:"revisionId"`
	Metadata       json.RawMessage `json:"metadata"`
	Provenance     json.RawMessage `json:"provenance"`
	Tags           []string        `json:"tags"`
	CreatedAt      time.Time       `json:"createdAt"`
	Score          float64         `json:"score"`
	MatchedChunks  []ChunkResult   `json:"matchedChunks"`
}

type ChunkResult struct {
	ID                 uuid.UUID       `json:"id"`
	Ordinal            int             `json:"ordinal"`
	Kind               string          `json:"kind"`
	Text               string          `json:"text"`
	Snippet            string          `json:"snippet"`
	StructuralLocation json.RawMessage `json:"structuralLocation"`
	KeywordRank        *int            `json:"keywordRank,omitempty"`
	KeywordScore       *float64        `json:"keywordScore,omitempty"`
	VectorRank         *int            `json:"vectorRank,omitempty"`
	VectorScore        *float64        `json:"vectorScore,omitempty"`
	Score              float64         `json:"score"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Search(ctx context.Context, projectID uuid.UUID, request Request, queryVector []float32) (Response, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return Response{}, fmt.Errorf("search query is required")
	}
	if request.Limit <= 0 {
		request.Limit = 20
	}
	request.Limit = min(request.Limit, 100)
	candidateLimit := min(max(request.Limit*8, 40), 200)
	keyword, err := r.keywordCandidates(ctx, projectID, request, candidateLimit)
	if err != nil {
		return Response{}, err
	}
	var vector []candidate
	if len(queryVector) > 0 {
		vector, err = r.vectorCandidates(ctx, projectID, request.Filters, queryVector, candidateLimit)
		if err != nil {
			return Response{}, err
		}
	}
	results := fuse(keyword, vector, request.Limit)
	if results == nil {
		results = make([]DocumentResult, 0)
	}
	return Response{
		Query: request.Query, Filters: request.Filters, Modes: Modes{Keyword: true, Vector: len(queryVector) > 0}, Results: results,
	}, nil
}

type candidate struct {
	chunkID        uuid.UUID
	ordinal        int
	kind           string
	text           string
	snippet        string
	location       json.RawMessage
	documentID     uuid.UUID
	sourceType     string
	sourceInstance string
	sourceIdentity string
	title          string
	revisionID     uuid.UUID
	metadata       json.RawMessage
	provenance     json.RawMessage
	tags           []string
	createdAt      time.Time
	rawScore       float64
}

func (r *Repository) keywordCandidates(ctx context.Context, projectID uuid.UUID, request Request, limit int) ([]candidate, error) {
	where, arguments := filterSQL(request.Filters, 3)
	arguments = append([]any{projectID, request.Query}, arguments...)
	limitPosition := len(arguments) + 1
	arguments = append(arguments, limit)
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.ordinal, c.content_kind, c.normalized_text,
			ts_headline('english', c.normalized_text, websearch_to_tsquery('english', $2),
				'StartSel=<mark>, StopSel=</mark>, MaxFragments=2, MaxWords=35, MinWords=12'),
			c.structural_location, d.id, d.source_type, si.external_key, d.source_identity,
			d.title, r.id, r.metadata, r.provenance,
			ARRAY(SELECT t.name FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
				WHERE dt.document_id = d.id AND dt.project_id = d.project_id ORDER BY t.name),
			r.created_at, ts_rank_cd(c.search_vector, websearch_to_tsquery('english', $2), 32)
		FROM chunks c
		JOIN revisions r ON r.id = c.revision_id AND r.project_id = c.project_id
		JOIN documents d ON d.current_revision_id = r.id AND d.project_id = c.project_id
		JOIN source_instances si ON si.id = d.source_instance_id AND si.project_id = d.project_id
		WHERE c.project_id = $1 AND d.deleted_at IS NULL
		  AND c.search_vector @@ websearch_to_tsquery('english', $2)`+where+`
		ORDER BY ts_rank_cd(c.search_vector, websearch_to_tsquery('english', $2), 32) DESC, c.id
		LIMIT $`+fmt.Sprint(limitPosition), arguments...)
	if err != nil {
		return nil, fmt.Errorf("keyword retrieval: %w", err)
	}
	return collectCandidates(rows)
}

func (r *Repository) vectorCandidates(ctx context.Context, projectID uuid.UUID, filters Filters, queryVector []float32, limit int) ([]candidate, error) {
	where, arguments := filterSQL(filters, 3)
	arguments = append([]any{projectID, vectorLiteral(queryVector)}, arguments...)
	limitPosition := len(arguments) + 1
	arguments = append(arguments, limit)
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.ordinal, c.content_kind, c.normalized_text, c.normalized_text,
			c.structural_location, d.id, d.source_type, si.external_key, d.source_identity,
			d.title, r.id, r.metadata, r.provenance,
			ARRAY(SELECT t.name FROM document_tags dt JOIN tags t ON t.id = dt.tag_id
				WHERE dt.document_id = d.id AND dt.project_id = d.project_id ORDER BY t.name),
			r.created_at, 1 - (e.embedding <=> $2::vector)
		FROM embeddings e
		JOIN chunks c ON c.id = e.chunk_id AND c.project_id = e.project_id
		JOIN revisions r ON r.id = c.revision_id AND r.project_id = c.project_id
		JOIN documents d ON d.current_revision_id = r.id AND d.project_id = c.project_id
		JOIN source_instances si ON si.id = d.source_instance_id AND si.project_id = d.project_id
		WHERE e.project_id = $1 AND d.deleted_at IS NULL AND e.model = 'voyage/voyage-4'`+where+`
		ORDER BY e.embedding <=> $2::vector, c.id
		LIMIT $`+fmt.Sprint(limitPosition), arguments...)
	if err != nil {
		return nil, fmt.Errorf("vector retrieval: %w", err)
	}
	return collectCandidates(rows)
}

func filterSQL(filters Filters, firstPosition int) (string, []any) {
	var clauses []string
	var arguments []any
	add := func(expression string, value any) {
		position := firstPosition + len(arguments)
		clauses = append(clauses, fmt.Sprintf(expression, position))
		arguments = append(arguments, value)
	}
	if len(filters.SourceTypes) > 0 {
		add("d.source_type = ANY($%d::text[])", filters.SourceTypes)
	}
	if len(filters.Branches) > 0 {
		add("r.metadata ->> 'branch' = ANY($%d::text[])", filters.Branches)
	}
	if len(filters.Repositories) > 0 {
		add("r.metadata ->> 'repository' = ANY($%d::text[])", filters.Repositories)
	}
	if len(filters.Tags) > 0 {
		add("EXISTS (SELECT 1 FROM document_tags fdt JOIN tags ft ON ft.id = fdt.tag_id AND ft.project_id = fdt.project_id WHERE fdt.document_id = d.id AND fdt.project_id = d.project_id AND ft.name = ANY($%d::text[]))", filters.Tags)
	}
	if filters.CreatedFrom != nil {
		add("r.created_at >= $%d", *filters.CreatedFrom)
	}
	if filters.CreatedTo != nil {
		add("r.created_at <= $%d", *filters.CreatedTo)
	}
	if len(clauses) == 0 {
		return "", arguments
	}
	return " AND " + strings.Join(clauses, " AND "), arguments
}

func collectCandidates(rows pgx.Rows) ([]candidate, error) {
	defer rows.Close()
	var result []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.chunkID, &item.ordinal, &item.kind, &item.text, &item.snippet,
			&item.location, &item.documentID, &item.sourceType, &item.sourceInstance, &item.sourceIdentity,
			&item.title, &item.revisionID, &item.metadata, &item.provenance, &item.tags, &item.createdAt,
			&item.rawScore); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func fuse(keyword, vector []candidate, limit int) []DocumentResult {
	type fusedChunk struct {
		candidate
		result ChunkResult
	}
	chunks := make(map[uuid.UUID]*fusedChunk, len(keyword)+len(vector))
	for rank, item := range keyword {
		entry := chunks[item.chunkID]
		if entry == nil {
			entry = &fusedChunk{candidate: item, result: chunkResult(item)}
			chunks[item.chunkID] = entry
		}
		position := rank + 1
		entry.result.KeywordRank = &position
		score := item.rawScore
		entry.result.KeywordScore = &score
		entry.result.Score += 1 / (reciprocalRankConstant + float64(position))
	}
	for rank, item := range vector {
		entry := chunks[item.chunkID]
		if entry == nil {
			entry = &fusedChunk{candidate: item, result: chunkResult(item)}
			chunks[item.chunkID] = entry
		}
		position := rank + 1
		entry.result.VectorRank = &position
		score := item.rawScore
		entry.result.VectorScore = &score
		entry.result.Score += 1 / (reciprocalRankConstant + float64(position))
	}
	documents := make(map[uuid.UUID]*DocumentResult)
	for _, chunk := range chunks {
		document := documents[chunk.documentID]
		if document == nil {
			document = &DocumentResult{
				ID: chunk.documentID, SourceType: chunk.sourceType, SourceInstance: chunk.sourceInstance,
				SourceIdentity: chunk.sourceIdentity, Title: chunk.title, RevisionID: chunk.revisionID,
				Metadata: chunk.metadata, Provenance: chunk.provenance, Tags: chunk.tags, CreatedAt: chunk.createdAt,
			}
			documents[chunk.documentID] = document
		}
		document.Score = max(document.Score, chunk.result.Score)
		document.MatchedChunks = append(document.MatchedChunks, chunk.result)
	}
	result := make([]DocumentResult, 0, len(documents))
	for _, document := range documents {
		sort.Slice(document.MatchedChunks, func(i, j int) bool {
			return document.MatchedChunks[i].Score > document.MatchedChunks[j].Score
		})
		result = append(result, *document)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Title < result[j].Title
		}
		return result[i].Score > result[j].Score
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func chunkResult(item candidate) ChunkResult {
	return ChunkResult{
		ID: item.chunkID, Ordinal: item.ordinal, Kind: item.kind, Text: item.text,
		Snippet: item.snippet, StructuralLocation: item.location,
	}
}

func vectorLiteral(vector []float32) string {
	var output strings.Builder
	output.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			output.WriteByte(',')
		}
		fmt.Fprintf(&output, "%g", value)
	}
	output.WriteByte(']')
	return output.String()
}
