import { writable, derived } from 'svelte/store';
import { auth as authApi } from '$api/client';
import type { User } from '$api/types';

function decodeJwtPayload(token: string): Record<string, unknown> | null {
	try {
		const base64 = token.split('.')[1];
		const json = atob(base64.replace(/-/g, '+').replace(/_/g, '/'));
		return JSON.parse(json);
	} catch {
		return null;
	}
}

function userFromToken(token: string): User | null {
	const payload = decodeJwtPayload(token);
	if (!payload) return null;

	const exp = payload.exp as number | undefined;
	if (exp && exp * 1000 < Date.now()) return null;

	return {
		id: payload.user_id as string,
		email: payload.email as string,
		role: payload.role as User['role'],
		created_at: ''
	};
}

function createAuthStore() {
	const { subscribe, set } = writable<User | null>(null);

	// Initialize from stored token
	const token = authApi.getToken();
	if (token) {
		const u = userFromToken(token);
		if (u) {
			set(u);
		} else {
			authApi.clearToken();
		}
	}

	return {
		subscribe,

		async login(email: string, password: string) {
			const res = await authApi.login(email, password);
			authApi.setToken(res.token);
			set(res.user);
		},

		async register(email: string, password: string, inviteCode?: string) {
			const res = await authApi.register(email, password, inviteCode);
			authApi.setToken(res.token);
			set(res.user);
		},

		logout() {
			authApi.logout().catch(() => {});
			authApi.clearToken();
			set(null);
		}
	};
}

export const user = createAuthStore();

export const isAdmin = derived(user, ($u) =>
	$u?.role === 'admin' || $u?.role === 'superadmin'
);
