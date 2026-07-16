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
  dependencyCount: number;
  dependentCount: number;
  openAnnotationCount: number;
  briefingCategory?: string;
  briefingHome?: boolean;
}

export type SourceType = "task" | "note" | "briefing" | "repository" | "conversation";

export interface RepositoryGroup {
  repository: string;
  branch: string;
  documents: DocumentSummary[];
}

export interface TermSummary {
  name: string;
  title: string;
  defined: boolean;
  definitionDocumentId?: string;
  referenceCount: number;
}

export interface TermReference {
  name: string;
  title: string;
  defined: boolean;
}

export interface BrowseResponse {
  project: ProjectSummary;
  sources: Array<{ id: string; sourceType: SourceType; sourceInstance: string; metadata: Json; documentCount: number; lastCompleteSyncAt?: string; updatedAt: string }>;
  tags: string[];
  terms: TermSummary[];
  tasks: DocumentSummary[];
  taskStatuses: string[];
  notes: DocumentSummary[];
  briefings: DocumentSummary[];
  repositories: RepositoryGroup[];
  conversations: DocumentSummary[];
  annotationCount: number;
  ingestionFailureCount: number;
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
  direction: "dependency" | "dependent" | "related";
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
  terms: TermReference[];
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
  replies: AnnotationReply[];
}

export interface AnnotationReply {
  id: string;
  annotationId: string;
  body: string;
  attributedUsername: string;
  createdAt: string;
  updatedAt: string;
}

export interface IngestionFailure {
  id: string;
  projectId: string;
  sourceType: SourceType;
  sourceInstance: string;
  path: string;
  message: string;
  createdAt: string;
  updatedAt: string;
}

export interface BriefingSetting {
  documentId: string;
  category: string;
  home: boolean;
  updatedAt?: string;
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
