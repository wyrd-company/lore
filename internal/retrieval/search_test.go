package retrieval

import (
	"testing"

	"github.com/google/uuid"
)

func TestReciprocalRankFusionRewardsAgreement(t *testing.T) {
	t.Parallel()
	agreedDocument := uuid.New()
	keywordOnlyDocument := uuid.New()
	agreedChunk := candidate{chunkID: uuid.New(), documentID: agreedDocument, title: "agreed", rawScore: 0.7}
	keywordOnly := candidate{chunkID: uuid.New(), documentID: keywordOnlyDocument, title: "keyword", rawScore: 0.9}
	results := fuse([]candidate{keywordOnly, agreedChunk}, []candidate{agreedChunk}, 10)
	if len(results) != 2 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].ID != agreedDocument {
		t.Fatalf("agreement did not win RRF: %#v", results)
	}
	chunk := results[0].MatchedChunks[0]
	if chunk.KeywordRank == nil || chunk.VectorRank == nil || chunk.Score <= results[1].MatchedChunks[0].Score {
		t.Fatalf("fused rank evidence missing: %#v", chunk)
	}
}
