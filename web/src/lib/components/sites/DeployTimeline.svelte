<script lang="ts">
	import type { Deployment } from '$api/types';
	import { deployments as deploymentsApi } from '$api/client';
	import Badge from '$components/ui/Badge.svelte';
	import Button from '$components/ui/Button.svelte';
	import { timeAgo } from '$lib/utils/time';
	import { Loader2 } from 'lucide-svelte';
	import { showError } from '$lib/utils/errors';

	interface Props {
		siteId: string;
		items: Deployment[];
		activeVersion: number | null;
		page: number;
		total: number;
		onRollback: () => void;
		onPageChange: (page: number) => void;
	}

	let { siteId, items, activeVersion, page, total, onRollback, onPageChange }: Props = $props();

	let rollingBack = $state<number | null>(null);

	async function handleRollback(version: number) {
		rollingBack = version;
		try {
			await deploymentsApi.rollback(siteId, version);
			onRollback();
		} catch (e) {
			showError(e);
		} finally {
			rollingBack = null;
		}
	}

	const perPage = 20;
	let totalPages = $derived(Math.ceil(total / perPage));
</script>

<div class="space-y-4">
	<div>
		<h3 class="text-lg font-semibold mb-1">Deployments</h3>
		<p class="text-sm text-text-muted">{total} total deployment{total !== 1 ? 's' : ''}</p>
	</div>

	{#if items.length === 0}
		<p class="text-sm text-text-muted py-8 text-center">No deployments yet.</p>
	{:else}
		<div class="relative pl-6">
			<!-- Timeline line -->
			<div class="absolute left-[9px] top-2 bottom-2 w-px bg-border"></div>

			{#each items as dep (dep.id)}
				{@const isActive = dep.version === activeVersion}
				<div class="relative flex items-start gap-4 pb-6 last:pb-0">
					<!-- Dot -->
					<div class="absolute left-[-15px] top-1.5">
						{#if isActive}
							<span class="relative flex size-3">
								<span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-40"></span>
								<span class="relative inline-flex size-3 rounded-full bg-success"></span>
							</span>
						{:else}
							<span class="flex size-3 rounded-full border-2 border-border bg-base"></span>
						{/if}
					</div>

					<!-- Content -->
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2 flex-wrap">
							<span class="font-semibold text-text">v{dep.version}</span>
							{#if isActive}
								<Badge variant="success">Live</Badge>
							{/if}
							{#if dep.has_worker}
								<Badge variant="info">Worker</Badge>
							{/if}
							<span class="ml-auto text-xs text-text-muted">{timeAgo(dep.uploaded_at)}</span>
						</div>
						<div class="flex items-center gap-2 mt-1">
							<code class="text-xs font-mono text-text-muted">{dep.file_hash.slice(0, 12)}</code>
							{#if !isActive}
								<Button
									variant="ghost"
									size="sm"
									onclick={() => handleRollback(dep.version)}
									disabled={rollingBack !== null}
								>
									{#if rollingBack === dep.version}
										<Loader2 class="size-3 animate-spin" />
									{:else}
										Rollback
									{/if}
								</Button>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<div class="flex items-center justify-center gap-2 pt-4">
				<Button variant="ghost" size="sm" disabled={page <= 1} onclick={() => onPageChange(page - 1)}>
					Prev
				</Button>
				<span class="text-xs text-text-muted">Page {page} of {totalPages}</span>
				<Button variant="ghost" size="sm" disabled={page >= totalPages} onclick={() => onPageChange(page + 1)}>
					Next
				</Button>
			</div>
		{/if}
	{/if}
</div>
