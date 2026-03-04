import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const STORAGE_KEY = 'hostedat_sidebar_collapsed';

function getInitial(): boolean {
	if (!browser) return false;
	return localStorage.getItem(STORAGE_KEY) === 'true';
}

export const sidebarCollapsed = writable<boolean>(getInitial());
export const sidebarMobileOpen = writable<boolean>(false);

if (browser) {
	sidebarCollapsed.subscribe(($collapsed) => {
		localStorage.setItem(STORAGE_KEY, String($collapsed));
	});
}
