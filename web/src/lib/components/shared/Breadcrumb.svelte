<script lang="ts">
	import { ChevronRight } from 'lucide-svelte';

	interface Crumb {
		label: string;
		href?: string;
	}

	interface Props {
		items: Crumb[];
	}

	let { items }: Props = $props();
</script>

<nav class="flex items-center gap-1 text-sm mb-4">
	{#each items as crumb, i}
		{#if i > 0}
			<ChevronRight class="size-3.5 text-text-muted" />
		{/if}
		{#if crumb.href}
			<a href={crumb.href} class="text-text-muted hover:text-text transition-colors">{crumb.label}</a>
		{:else}
			<span class="text-text font-medium">{crumb.label}</span>
		{/if}
	{/each}
</nav>
