<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		variant?: 'default' | 'success' | 'warning' | 'error' | 'info' | 'outline';
		class?: string;
		children: Snippet;
	}

	let { variant = 'default', class: className = '', children }: Props = $props();

	const variants: Record<string, string> = {
		default: 'bg-elevated text-text-muted',
		success: 'bg-success/15 text-success',
		warning: 'bg-warning/15 text-warning',
		error: 'bg-error/15 text-error',
		info: 'bg-info/15 text-info',
		outline: 'border border-border text-text-muted'
	};
</script>

<span class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium {variants[variant]} {className}">
	{@render children()}
</span>
