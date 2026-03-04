<script lang="ts">
	import type { AnalyticsSummary } from '$api/types';
	import { formatNumber, formatBytes } from '$lib/utils/time';

	interface Props { data: AnalyticsSummary; }
	let { data }: Props = $props();

	const cards = $derived([
		{ label: 'Requests', value: formatNumber(data.requests) },
		{ label: 'Unique Visitors', value: formatNumber(data.unique_visitors) },
		{ label: 'Bandwidth', value: formatBytes(data.bytes_sent) },
		{ label: '2xx', value: formatNumber(data.status_2xx), color: 'text-success' },
		{ label: '4xx', value: formatNumber(data.status_4xx), color: 'text-warning' },
		{ label: '5xx', value: formatNumber(data.status_5xx), color: 'text-error' }
	]);
</script>

<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
	{#each cards as card}
		<div class="rounded-lg border border-border bg-base p-3">
			<p class="text-xs text-text-muted">{card.label}</p>
			<p class="text-lg font-semibold tabular-nums {card.color ?? 'text-text'}">{card.value}</p>
		</div>
	{/each}
</div>
