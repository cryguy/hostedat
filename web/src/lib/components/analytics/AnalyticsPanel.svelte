<script lang="ts">
	import type { AnalyticsSummary, TimeseriesPoint, TopEntry } from '$api/types';
	import { analytics } from '$api/client';
	import SummaryCards from './SummaryCards.svelte';
	import TimeseriesChart from './TimeseriesChart.svelte';
	import TopTable from './TopTable.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	interface Props { siteId: string; }
	let { siteId }: Props = $props();

	type Period = '24h' | '7d' | '30d';
	const periods: { value: Period; label: string }[] = [
		{ value: '24h', label: '24 hours' },
		{ value: '7d', label: '7 days' },
		{ value: '30d', label: '30 days' }
	];

	let period = $state<Period>('24h');
	let loading = $state(true);
	let summary = $state<AnalyticsSummary | null>(null);
	let timeseries = $state<TimeseriesPoint[]>([]);
	let pages = $state<TopEntry[]>([]);
	let referrers = $state<TopEntry[]>([]);

	async function load() {
		loading = true;
		try {
			const [s, t, p, r] = await Promise.all([
				analytics.summary(siteId, period),
				analytics.timeseries(siteId, period),
				analytics.pages(siteId, period),
				analytics.referrers(siteId, period)
			]);
			summary = s;
			timeseries = t;
			pages = p;
			referrers = r;
		} catch (e) {
			showError(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function changePeriod(p: Period) {
		period = p;
		load();
	}
</script>

<div class="space-y-6">
	<!-- Period selector -->
	<div class="flex items-center gap-1 rounded-lg border border-border bg-base p-1 w-fit">
		{#each periods as p}
			<button
				onclick={() => changePeriod(p.value)}
				class="px-3 py-1.5 rounded-md text-xs font-medium transition-colors
					{period === p.value
						? 'bg-primary text-white'
						: 'text-text-muted hover:text-text'}"
			>
				{p.label}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="space-y-4">
			<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
				{#each Array(6) as _}
					<Skeleton class="h-16 rounded-lg" />
				{/each}
			</div>
			<Skeleton class="h-48 rounded-lg" />
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<Skeleton class="h-40 rounded-lg" />
				<Skeleton class="h-40 rounded-lg" />
			</div>
		</div>
	{:else if summary}
		<SummaryCards data={summary} />
		<TimeseriesChart points={timeseries} />
		<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
			<TopTable title="Top Pages" items={pages} emptyLabel="No page data" />
			<TopTable title="Top Referrers" items={referrers} emptyLabel="No referrer data" />
		</div>
	{:else}
		<p class="text-sm text-text-muted py-8 text-center">No analytics data available yet.</p>
	{/if}
</div>
