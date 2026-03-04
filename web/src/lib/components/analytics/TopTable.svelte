<script lang="ts">
	import type { TopEntry } from '$api/types';
	import { formatNumber } from '$lib/utils/time';

	interface Props {
		title: string;
		items: TopEntry[];
		emptyLabel?: string;
	}

	let { title, items, emptyLabel = 'No data' }: Props = $props();

	const maxReqs = $derived(Math.max(...items.map((e) => e.requests), 1));
</script>

<div class="rounded-lg border border-border bg-base p-4">
	<h4 class="text-sm font-semibold mb-3">{title}</h4>

	{#if items.length === 0}
		<p class="text-sm text-text-muted py-4 text-center">{emptyLabel}</p>
	{:else}
		<div class="space-y-1.5">
			{#each items as entry}
				{@const pct = (entry.requests / maxReqs) * 100}
				<div class="relative rounded px-2 py-1.5">
					<div
						class="absolute inset-0 bg-primary/10 rounded"
						style="width: {pct}%"
					></div>
					<div class="relative flex items-center justify-between text-xs">
						<span class="font-mono text-text truncate mr-2">{entry.value || '(direct)'}</span>
						<span class="text-text-muted tabular-nums shrink-0">{formatNumber(entry.requests)}</span>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
