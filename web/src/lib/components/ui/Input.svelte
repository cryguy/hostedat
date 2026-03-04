<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';

	interface Props extends HTMLInputAttributes {
		class?: string;
	}

	let { class: className = '', ...rest }: Props = $props();
</script>

<input
	class="h-9 w-full rounded-lg border border-border bg-surface px-3 text-sm text-text placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-primary/40 focus:border-primary disabled:opacity-50 transition-colors {className}"
	{...rest}
/>
