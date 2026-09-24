// Domain types mirror the paperless OpenAPI responses (see api/openapi/paperless.yaml)
// and the service read projections (internal/documents, internal/categories,
// internal/search, internal/permissions, internal/stats). Content and extracted
// metadata are never returned in list/read projections and are omitted here.

export type DocumentStatus = 'active' | 'archived' | 'deleted'
export type DocumentSource = 'upload' | 'email'
export type ProcessingStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'retrying'

export type ResourceType = 'document' | 'category'
export type SubjectType = 'user' | 'role' | 'tenant'
export type Relation = 'owner' | 'editor' | 'viewer' | 'sharer'

export interface Document {
  id: string
  tenant_id: string
  category_id?: string | undefined
  category_path?: string | undefined
  name: string
  description?: string | undefined
  file_name: string
  file_size: number
  mime_type: string
  checksum: string
  status: DocumentStatus
  source: DocumentSource
  tags?: Record<string, string> | undefined
  processing_status: ProcessingStatus
  created_by: string
  created_at: string
  updated_at: string
}

export interface DocumentInput {
  name?: string | undefined
  description?: string | undefined
  category_id?: string | undefined
  tags?: Record<string, string> | undefined
  status?: DocumentStatus | undefined
}

export interface BatchResult {
  id: string
  ok: boolean
  error?: string
}

export interface Category {
  id: string
  parent_id: string | null
  name: string
  path: string
  description?: string
  depth: number
  sort_order: number
  document_count: number
  subcategory_count: number
}

export interface CategoryInput {
  name: string
  parent_id?: string | undefined
  description?: string | undefined
  sort_order?: number | undefined
}

export interface CategoryTreeNode {
  category: Category
  children: CategoryTreeNode[]
}

export interface PermissionTuple {
  id: string
  resource_type: ResourceType
  resource_id: string
  subject_type: SubjectType
  subject_id: string
  relation: Relation
  granted_by?: string
  expires_at?: string | null
}

export interface GrantInput {
  resource_type: ResourceType
  resource_id: string
  subject_type: SubjectType
  subject_id: string
  relation: Relation
  expires_at?: string | null
}

// EffectivePermissions mirrors authz.Permissions (the /permissions/effective response).
export interface EffectivePermissions {
  read: boolean
  write: boolean
  delete: boolean
  share: boolean
  download: boolean
}

export interface SearchHit {
  id: string
  name: string
  category_id?: string
  category_path?: string
  mime_type: string
  status?: string
  rank: number
  snippet: string
}

// Stats mirrors internal/stats.Snapshot (the /statistics/tenant response).
export interface Stats {
  documents_by_status: Record<string, number>
  documents_by_source: Record<string, number>
  documents_by_mime: Record<string, number>
  storage_bytes: number
  storage_by_category: Record<string, number>
  categories_total: number
  documents_total: number
  backlog: Record<string, number>
}
