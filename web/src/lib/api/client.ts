import type {
	AuthResponse,
	Site,
	Deployment,
	DeploymentsListResponse,
	APIKey,
	APIKeyCreateResponse,
	Invite,
	InstanceSettings,
	User,
	WorkerEnvVar,
	KVNamespace,
	CronSchedule,
	WorkerLog,
	D1Database,
	DurableObjectNamespace,
	StorageBucket,
	S3Credential,
	AnalyticsSummary,
	TimeseriesPoint,
	TopEntry,
	AuditLog,
	AuditLogParams,
	PaginatedResponse
} from './types';

const TOKEN_KEY = 'hostedat_token';

function getToken(): string | null {
	return sessionStorage.getItem(TOKEN_KEY);
}

function setToken(token: string) {
	sessionStorage.setItem(TOKEN_KEY, token);
}

function clearToken() {
	sessionStorage.removeItem(TOKEN_KEY);
}

function toQueryString(params?: Record<string, string | number | undefined>): string {
	if (!params) return '';
	const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== '');
	if (entries.length === 0) return '';
	return '?' + entries.map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`).join('&');
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const token = getToken();
	const headers: Record<string, string> = {
		...(options.headers as Record<string, string>)
	};

	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	if (!(options.body instanceof FormData)) {
		headers['Content-Type'] = 'application/json';
	}

	const res = await fetch(`/api/v1${path}`, { ...options, headers });

	if (res.status === 401) {
		clearToken();
		window.location.href = '/login';
		throw new Error('Unauthorized');
	}

	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: 'Request failed' }));
		throw new Error(body.error || 'Request failed');
	}

	if (res.status === 204) return undefined as T;

	return res.json();
}

export const auth = {
	getToken,
	setToken,
	clearToken,

	login(email: string, password: string) {
		return request<AuthResponse>('/auth/login', {
			method: 'POST',
			body: JSON.stringify({ email, password })
		});
	},

	register(email: string, password: string, inviteCode?: string) {
		return request<AuthResponse>('/auth/register', {
			method: 'POST',
			body: JSON.stringify({
				email,
				password,
				...(inviteCode ? { invite_code: inviteCode } : {})
			})
		});
	},

	logout() {
		return request<{ message: string }>('/auth/logout', { method: 'POST' });
	}
};

export const sites = {
	list: () => request<Site[]>('/sites'),
	get: (id: string) => request<Site>(`/sites/${id}`),
	create: (name: string, subdomainSlug?: string) =>
		request<Site>('/sites', {
			method: 'POST',
			body: JSON.stringify({ name, ...(subdomainSlug ? { subdomain_slug: subdomainSlug } : {}) })
		}),
	update: (id: string, data: { name?: string; spa_mode?: boolean }) =>
		request<Site>(`/sites/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
	delete: (id: string) => request<{ message: string }>(`/sites/${id}`, { method: 'DELETE' })
};

export const deployments = {
	list: (siteId: string, page = 1) =>
		request<DeploymentsListResponse>(`/sites/${siteId}/deployments?page=${page}`),
	deploy: (siteId: string, file: File) => {
		const form = new FormData();
		form.append('file', file);
		return request<Deployment>(`/sites/${siteId}/deploy`, { method: 'POST', body: form });
	},
	rollback: (siteId: string, version: number) =>
		request<{ message: string; active_version: number }>(
			`/sites/${siteId}/deployments/${version}/rollback`,
			{ method: 'POST' }
		)
};

export const apiKeys = {
	list: () => request<APIKey[]>('/keys'),
	create: (name: string) =>
		request<APIKeyCreateResponse>('/keys', { method: 'POST', body: JSON.stringify({ name }) }),
	delete: (id: string) => request<{ message: string }>(`/keys/${id}`, { method: 'DELETE' })
};

export const workers = {
	listEnv: (siteId: string) => request<WorkerEnvVar[]>(`/sites/${siteId}/worker/env`),
	setEnv: (siteId: string, data: { name: string; value: string; secret: boolean }) =>
		request<WorkerEnvVar>(`/sites/${siteId}/worker/env`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),
	deleteEnv: (siteId: string, varId: string) =>
		request<void>(`/sites/${siteId}/worker/env/${varId}`, { method: 'DELETE' }),

	listKV: (siteId: string) => request<KVNamespace[]>(`/sites/${siteId}/worker/kv`),
	createKV: (siteId: string, name: string) =>
		request<KVNamespace>(`/sites/${siteId}/worker/kv`, {
			method: 'POST',
			body: JSON.stringify({ name })
		}),
	deleteKV: (siteId: string, nsId: string) =>
		request<void>(`/sites/${siteId}/worker/kv/${nsId}`, { method: 'DELETE' }),

	listCrons: (siteId: string) => request<CronSchedule[]>(`/sites/${siteId}/worker/crons`),
	createCron: (siteId: string, data: { cron: string; enabled: boolean }) =>
		request<CronSchedule>(`/sites/${siteId}/worker/crons`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),
	deleteCron: (siteId: string, cronId: string) =>
		request<void>(`/sites/${siteId}/worker/crons/${cronId}`, { method: 'DELETE' }),

	getLogs: (siteId: string) => request<WorkerLog[]>(`/sites/${siteId}/worker/logs`),

	listD1: (siteId: string) =>
		request<PaginatedResponse<D1Database>>(`/sites/${siteId}/worker/d1`),
	createD1: (siteId: string, name: string) =>
		request<D1Database>(`/sites/${siteId}/worker/d1`, {
			method: 'POST',
			body: JSON.stringify({ name })
		}),
	deleteD1: (siteId: string, d1Id: string) =>
		request<void>(`/sites/${siteId}/worker/d1/${d1Id}`, { method: 'DELETE' }),

	listDurableObjects: (siteId: string) =>
		request<PaginatedResponse<DurableObjectNamespace>>(`/sites/${siteId}/worker/do`),
	createDurableObject: (siteId: string, name: string) =>
		request<DurableObjectNamespace>(`/sites/${siteId}/worker/do`, {
			method: 'POST',
			body: JSON.stringify({ name })
		}),
	deleteDurableObject: (siteId: string, doId: string) =>
		request<void>(`/sites/${siteId}/worker/do/${doId}`, { method: 'DELETE' })
};

export const storage = {
	listBuckets: (siteId: string) =>
		request<StorageBucket[]>(`/sites/${siteId}/storage/buckets`),
	createBucket: (siteId: string, data: { name: string; bucket_name: string; public?: boolean }) =>
		request<StorageBucket>(`/sites/${siteId}/storage/buckets`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),
	updateBucket: (siteId: string, bucketId: string, data: { public: boolean }) =>
		request<StorageBucket>(`/sites/${siteId}/storage/buckets/${bucketId}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),
	deleteBucket: (siteId: string, bucketId: string) =>
		request<void>(`/sites/${siteId}/storage/buckets/${bucketId}`, { method: 'DELETE' })
};

export const s3Credentials = {
	list: () => request<S3Credential[]>('/s3-credentials'),
	create: (name: string) =>
		request<S3Credential & { secret_access_key: string }>('/s3-credentials', {
			method: 'POST',
			body: JSON.stringify({ name })
		}),
	delete: (id: string) => request<void>(`/s3-credentials/${id}`, { method: 'DELETE' })
};

export const analytics = {
	summary: (siteId: string, period = '24h') =>
		request<AnalyticsSummary>(`/sites/${siteId}/analytics/summary?period=${period}`),
	timeseries: (siteId: string, period = '24h', bucket?: string) =>
		request<TimeseriesPoint[]>(
			`/sites/${siteId}/analytics/timeseries?period=${period}${bucket ? `&bucket=${bucket}` : ''}`
		),
	pages: (siteId: string, period = '24h', limit = 10) =>
		request<TopEntry[]>(`/sites/${siteId}/analytics/pages?period=${period}&limit=${limit}`),
	referrers: (siteId: string, period = '24h', limit = 10) =>
		request<TopEntry[]>(`/sites/${siteId}/analytics/referrers?period=${period}&limit=${limit}`)
};

export const auditLogs = {
	list: (params?: AuditLogParams) =>
		request<PaginatedResponse<AuditLog>>(`/audit-logs${toQueryString(params as Record<string, string | number | undefined>)}`)
};

export const admin = {
	listUsers: (page = 1) =>
		request<{ users: User[]; total: number; page: number }>(`/admin/users?page=${page}`),
	updateUserRole: (id: string, role: string) =>
		request<User>(`/admin/users/${id}/role`, { method: 'PATCH', body: JSON.stringify({ role }) }),
	deleteUser: (id: string) =>
		request<{ message: string }>(`/admin/users/${id}`, { method: 'DELETE' }),
	getSettings: () => request<InstanceSettings>('/admin/settings'),
	updateSettings: (data: Partial<InstanceSettings>) =>
		request<InstanceSettings>('/admin/settings', { method: 'PATCH', body: JSON.stringify(data) }),
	listInvites: () => request<Invite[]>('/admin/invites'),
	createInvite: (data: { max_uses?: number; expires_at?: string }) =>
		request<Invite>('/admin/invites', { method: 'POST', body: JSON.stringify(data) }),
	revokeInvite: (id: string) =>
		request<{ message: string }>(`/admin/invites/${id}`, { method: 'DELETE' })
};
