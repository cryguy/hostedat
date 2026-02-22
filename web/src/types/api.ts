export interface User {
  id: string
  email: string
  role: "superadmin" | "admin" | "user"
  invited_by?: string
  created_at: string
}

export interface Site {
  id: string
  user_id: string
  subdomain_slug: string
  name: string
  spa_mode: boolean
  has_worker: boolean
  active_version: number | null
  active_deploy_id?: string
  created_at: string
}

export interface Deployment {
  id: string
  site_id: string
  version: number
  file_hash: string
  has_worker: boolean
  uploaded_at: string
}

export interface APIKey {
  id: string
  user_id: string
  name: string
  last_used_at?: string
  created_at: string
}

export interface APIKeyCreateResponse {
  id: string
  name: string
  key: string
  created_at: string
}

export interface Invite {
  id: string
  code: string
  created_by: string
  max_uses?: number
  use_count: number
  expires_at?: string
  active: boolean
  created_at: string
}

export interface AuthResponse {
  token: string
  user: User
}

export interface DeploymentsListResponse {
  deployments: Deployment[]
  total: number
  page: number
}

export interface UsersListResponse {
  users: User[]
  total: number
  page: number
}

export interface InstanceSettings {
  registration_enabled: boolean
  invite_required: boolean
}

export interface WorkerEnvVar {
  id: string
  site_id: string
  name: string
  value: string
  secret: boolean
}

export interface KVNamespace {
  id: string
  site_id: string
  name: string
  created_at: string
}

export interface CronSchedule {
  id: string
  site_id: string
  cron: string
  enabled: boolean
  last_run_at?: string
  created_at: string
}

export interface WorkerLog {
  id: string
  site_id: string
  level: string
  message: string
  created_at: string
}

export interface StorageBucket {
  id: string
  site_id: string
  name: string
  bucket_name: string
  public: boolean
  created_at: string
}

export interface D1Database {
  id: string
  site_id: string
  name: string
  database_id: string
  created_at: string
}

export interface D1DatabaseListResponse {
  items: D1Database[]
  total: number
}

export interface DurableObjectNamespace {
  id: string
  site_id: string
  name: string
  namespace_id: string
  created_at: string
}

export interface DurableObjectNamespaceListResponse {
  items: DurableObjectNamespace[]
  total: number
}

export interface S3Credential {
  id: string
  user_id: string
  access_key_id: string
  external_key_id: string
  name: string
  last_used_at?: string
  created_at: string
}
