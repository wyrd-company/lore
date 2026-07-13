export type Json = Record<string, unknown>;

export interface ProjectSummary {
  id: string;
  slug: string;
  name: string;
  documentCount: number;
  sourceCount: number;
  sourceTypeCounts: Record<string, number>;
}

export interface DocumentSummary {
  id: string;
  sourceType: SourceType;
  sourceInstance: string;
  sourceIdentity: string;
  title: string;
  revisionId: string;
  metadata: Json;
  tags: string[];
  createdAt: string;
  updatedAt: string;
  chunkCount: number;
  embeddedChunkCount: number;
}

export type SourceType = "task" | "note" | "briefing" | "repository" | "conversation";

export interface RepositoryGroup {
  repository: string;
  branch: string;
  documents: DocumentSummary[];
}

export interface BrowseResponse {
  project: ProjectSummary;
  sources: Array<{ id: string; sourceType: SourceType; sourceInstance: string; metadata: Json; documentCount: number; lastCompleteSyncAt?: string; updatedAt: string }>;
  tags: string[];
  tasks: DocumentSummary[];
  notes: DocumentSummary[];
  briefings: DocumentSummary[];
  repositories: RepositoryGroup[];
  conversations: DocumentSummary[];
}

export interface RevisionSummary {
  id: string;
  contentHash: string;
  renderer: string;
  createdAt: string;
  current: boolean;
  chunkCount: number;
  embeddedChunks: number;
  annotationCount: number;
}

export interface Relationship {
  direction: "dependency" | "dependent";
  type: string;
  documentId: string;
  sourceIdentity: string;
  title: string;
  metadata: Json;
}

export interface DocumentDetail extends DocumentSummary {
  contentHash: string;
  normalizedText: string;
  renderedContent: string;
  renderer: string;
  provenance: Json;
  relationships: Relationship[];
  revisions: RevisionSummary[];
}

export interface RevisionDetail extends RevisionSummary {
  documentId: string;
  documentTitle: string;
  sourceType: SourceType;
  sourceIdentity: string;
  normalizedText: string;
  renderedContent: string;
  metadata: Json;
  provenance: Json;
}

export interface Annotation {
  id: string;
  projectId: string;
  documentId: string;
  documentIdentity: string;
  documentTitle: string;
  sourceType: SourceType;
  sourceInstance: string;
  revisionId?: string;
  revisionIdentity: string;
  body: string;
  status: "open" | "resolved" | "dismissed";
  attributedUsername: string;
  updatedBy: string;
  originatingOperation: string;
  selector: Json;
  selectedQuote?: string;
  quotePrefix?: string;
  quoteSuffix?: string;
  structuralLocation: Json;
  originalContentHash: string;
  sourceProvenance: Json;
  copiedFromAnnotationId?: string;
  priorTarget?: Json;
  resolvedAt?: string;
  resolvedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SearchResponse {
  query: string;
  filters: Json;
  modes: { keyword: boolean; vector: boolean };
  warnings?: string[];
  results: Array<DocumentSummary & {
    provenance: Json;
    score: number;
    matchedChunks: Array<{ id: string; ordinal: number; kind: string; text: string; snippet: string; structuralLocation: Json; keywordRank?: number; keywordScore?: number; vectorRank?: number; vectorScore?: number; score: number }>;
  }>;
}
