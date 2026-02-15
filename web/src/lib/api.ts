import type {
  AuthResponse,
  Site,
  Deployment,
  APIKey,
  APIKeyCreateResponse,
  Invite,
  UsersListResponse,
  InstanceSettings,
  User,
  WorkerEnvVar,
  KVNamespace,
  CronSchedule,
  WorkerLog,
} from "@/types/api"

const TOKEN_KEY = "hostedat_token"

function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  }

  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  if (!(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json"
  }

  const res = await fetch(`/api/v1${path}`, {
    ...options,
    headers,
  })

  if (res.status === 401) {
    clearToken()
    window.location.href = "/login"
    throw new Error("Unauthorized")
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: "Request failed" }))
    throw new Error(body.error || "Request failed")
  }

  if (res.status === 204) return undefined as T

  return res.json()
}

export const auth = {
  login(email: string, password: string) {
    return request<AuthResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    })
  },

  register(email: string, password: string, inviteCode?: string) {
    return request<AuthResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify({
        email,
        password,
        ...(inviteCode ? { invite_code: inviteCode } : {}),
      }),
    })
  },

  logout() {
    return request<{ message: string }>("/auth/logout", { method: "POST" })
  },

  getToken,
  setToken,
  clearToken,
}

export const sites = {
  list() {
    return request<Site[]>("/sites")
  },

  get(id: string) {
    return request<Site>(`/sites/${id}`)
  },

  create(name: string, subdomainSlug?: string) {
    return request<Site>("/sites", {
      method: "POST",
      body: JSON.stringify({
        name,
        ...(subdomainSlug ? { subdomain_slug: subdomainSlug } : {}),
      }),
    })
  },

  update(id: string, data: { name?: string; spa_mode?: boolean }) {
    return request<Site>(`/sites/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    })
  },

  delete(id: string) {
    return request<{ message: string }>(`/sites/${id}`, { method: "DELETE" })
  },
}

export const deployments = {
  list(siteId: string) {
    return request<Deployment[]>(`/sites/${siteId}/deployments`)
  },

  deploy(siteId: string, file: File) {
    const form = new FormData()
    form.append("file", file)
    return request<Deployment>(`/sites/${siteId}/deploy`, {
      method: "POST",
      body: form,
    })
  },

  rollback(siteId: string, version: number) {
    return request<{ message: string; active_version: number }>(
      `/sites/${siteId}/deployments/${version}/rollback`,
      { method: "POST" },
    )
  },
}

export const apiKeys = {
  list() {
    return request<APIKey[]>("/keys")
  },

  create(name: string) {
    return request<APIKeyCreateResponse>("/keys", {
      method: "POST",
      body: JSON.stringify({ name }),
    })
  },

  delete(id: string) {
    return request<{ message: string }>(`/keys/${id}`, { method: "DELETE" })
  },
}

export const admin = {
  listUsers(page = 1) {
    return request<UsersListResponse>(`/admin/users?page=${page}`)
  },

  updateUserRole(id: string, role: string) {
    return request<User>(`/admin/users/${id}/role`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    })
  },

  deleteUser(id: string) {
    return request<{ message: string }>(`/admin/users/${id}`, {
      method: "DELETE",
    })
  },

  getSettings() {
    return request<InstanceSettings>("/admin/settings")
  },

  updateSettings(data: Partial<InstanceSettings>) {
    return request<InstanceSettings>("/admin/settings", {
      method: "PATCH",
      body: JSON.stringify(data),
    })
  },

  listInvites() {
    return request<Invite[]>("/admin/invites")
  },

  createInvite(data: { max_uses?: number; expires_at?: string }) {
    return request<Invite>("/admin/invites", {
      method: "POST",
      body: JSON.stringify(data),
    })
  },

  revokeInvite(id: string) {
    return request<{ message: string }>(`/admin/invites/${id}`, {
      method: "DELETE",
    })
  },
}

export const workers = {
  listEnv(siteId: string) {
    return request<WorkerEnvVar[]>(`/sites/${siteId}/worker/env`)
  },
  setEnv(siteId: string, data: { name: string; value: string; secret: boolean }) {
    return request<WorkerEnvVar>(`/sites/${siteId}/worker/env`, {
      method: "POST",
      body: JSON.stringify(data),
    })
  },
  deleteEnv(siteId: string, varId: string) {
    return request<{ message: string }>(`/sites/${siteId}/worker/env/${varId}`, { method: "DELETE" })
  },
  listKV(siteId: string) {
    return request<KVNamespace[]>(`/sites/${siteId}/worker/kv`)
  },
  createKV(siteId: string, name: string) {
    return request<KVNamespace>(`/sites/${siteId}/worker/kv`, {
      method: "POST",
      body: JSON.stringify({ name }),
    })
  },
  deleteKV(siteId: string, nsId: string) {
    return request<{ message: string }>(`/sites/${siteId}/worker/kv/${nsId}`, { method: "DELETE" })
  },
  listCrons(siteId: string) {
    return request<CronSchedule[]>(`/sites/${siteId}/worker/crons`)
  },
  createCron(siteId: string, data: { cron: string; enabled: boolean }) {
    return request<CronSchedule>(`/sites/${siteId}/worker/crons`, {
      method: "POST",
      body: JSON.stringify(data),
    })
  },
  deleteCron(siteId: string, cronId: string) {
    return request<{ message: string }>(`/sites/${siteId}/worker/crons/${cronId}`, { method: "DELETE" })
  },
  getLogs(siteId: string) {
    return request<WorkerLog[]>(`/sites/${siteId}/worker/logs`)
  },
}
