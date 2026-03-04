<script lang="ts">
	import type { TimeseriesPoint } from '$api/types';
	import { formatNumber } from '$lib/utils/time';

	interface Props { points: TimeseriesPoint[]; }
	let { points }: Props = $props();

	const maxRequests = $derived(Math.max(...points.map((p) => p.requests), 1));

	function formatBucket(iso: string): string {
		const d = new Date(iso);
		// If points are hourly, show time; if daily, show date
		if (points.length > 7) {
			return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
		}
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
	}
</script>

<div class="rounded-lg border border-border bg-base p-4">
	<h4 class="text-sm font-semibold mb-3">Requests over time</h4>

	{#if points.length === 0}
		<p class="text-sm text-text-muted py-8 text-center">No data for this period.</p>
	{:else}
		<div class="flex items-end gap-px h-40">
			{#each points as point}
				{@const pct = (point.requests / maxRequests) * 100}
				<div class="flex-1 group relative flex flex-col items-center justify-end h-full min-w-0">
					<div
						class="w-full bg-primary/70 hover:bg-primary rounded-t transition-colors min-h-[2px]"
						style="height: {Math.max(pct, 1)}%"
					></div>
					<!-- Tooltip -->
					<div class="absolute bottom-full mb-2 hidden group-hover:block z-10">
						<div class="bg-elevated border border-border rounded-lg px-2 py-1.5 text-xs shadow-lg whitespace-nowrap">
							<p class="font-medium text-text">{formatNumber(point.requests)} requests</p>
							<p class="text-text-muted">{formatNumber(point.unique_visitors)} visitors</p>
							<p class="text-text-muted mt-0.5">{formatBucket(point.bucket)}</p>
						</div>
					</div>
				</div>
			{/each}
		</div>
		<!-- X-axis labels -->
		<div class="flex justify-between mt-1.5 text-[10px] text-text-muted">
			<span>{formatBucket(points[0].bucket)}</span>
			{#if points.length > 2}
				<span>{formatBucket(points[Math.floor(points.length / 2)].bucket)}</span>
			{/if}
			<span>{formatBucket(points[points.length - 1].bucket)}</span>
		</div>
	{/if}
</div>
