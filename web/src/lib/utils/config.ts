import { browser } from '$app/environment';

/** Returns the instance domain derived from the current hostname. */
export function getInstanceDomain(): string {
	if (!browser) return 'localhost';
	return window.location.hostname;
}
