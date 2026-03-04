/** Returns a human-readable relative time string like "2h ago", "3d ago". */
export function timeAgo(date: string | Date): string {
	const now = Date.now();
	const then = typeof date === 'string' ? new Date(date).getTime() : date.getTime();
	const seconds = Math.floor((now - then) / 1000);

	if (seconds < 60) return 'just now';
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	if (days < 30) return `${days}d ago`;
	const months = Math.floor(days / 30);
	if (months < 12) return `${months}mo ago`;
	const years = Math.floor(months / 12);
	return `${years}y ago`;
}

/** Formats bytes into a human-readable string like "1.2 MB". */
export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const i = Math.floor(Math.log(bytes) / Math.log(1024));
	const value = bytes / Math.pow(1024, i);
	return `${value >= 10 ? Math.round(value) : value.toFixed(1)} ${units[i]}`;
}

/** Formats a number with commas: 12847 → "12,847". */
export function formatNumber(n: number): string {
	return n.toLocaleString('en-US');
}
