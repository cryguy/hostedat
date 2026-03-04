import { toast } from 'svelte-sonner';

/** Extract a human-readable message from an unknown error and show a toast. */
export function showError(err: unknown, fallback = 'Something went wrong') {
	const message = err instanceof Error ? err.message : fallback;
	toast.error(message);
}
