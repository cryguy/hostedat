import { writable, derived } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'hostedat_theme';

function getInitialTheme(): Theme {
	if (!browser) return 'dark';
	const stored = localStorage.getItem(STORAGE_KEY) as Theme | null;
	return stored || 'dark';
}

function getSystemTheme(): 'dark' | 'light' {
	if (!browser) return 'dark';
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export const theme = writable<Theme>(getInitialTheme());

export const resolvedTheme = derived(theme, ($theme) =>
	$theme === 'system' ? getSystemTheme() : $theme
);

// Apply theme to document and persist
if (browser) {
	theme.subscribe(($theme) => {
		localStorage.setItem(STORAGE_KEY, $theme);
		const resolved = $theme === 'system' ? getSystemTheme() : $theme;
		document.documentElement.classList.toggle('dark', resolved === 'dark');
	});
}
