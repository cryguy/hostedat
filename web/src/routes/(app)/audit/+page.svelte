<script lang="ts">
	import { auditLogs } from '$api/client';
	import type { AuditLog, AuditLogParams } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import Button from '$components/ui/Button.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { ChevronLeft, ChevronRight, Filter, X } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	let items = $state<AuditLog[]>([]);
	let total = $state(0);
	let page = $state(1);
	let loading = $state(true);

	// Filters
	let filterAction = $state('');
	let filterResourceType = $state('');
	let showFilters = $state(false);

	const perPage = 50;
	const totalPages = $derived(Math.ceil(total / perPage));

	const actionGroups: Record<string, string[]> = {
		Auth: ['user.login', 'user.register', 'user.logout'],
		Sites: ['site.create', 'site.update', 'site.delete'],
		Deploy: ['deployment.create', 'deployment.activate', 'deployment.rollback'],
		Worker: ['worker.env.set', 'worker.env.delete', 'worker.cron.create', 'worker.cron.delete'],
		Storage: ['storage.bucket.create', 'storage.bucket.delete', 'storage.bucket.update'],
		Keys: ['api_key.create', 'api_key.delete'],
		Admin: ['admin.invite.create', 'admin.invite.delete', 'admin.user.update_role', 'admin.user.delete']
	};

	const resourceTypes = ['site', 'deployment', 'user', 'api_key', 'invite', 'worker', 'storage_bucket', 'kv_namespace', 'cron_schedule', 'd1_database', 'durable_object'];

	async function load() {
		loading = true;
		try {
			const params: AuditLogParams = { page };
			if (filterAction) params.action = filterAction;
			if (filterResourceType) params.resource_type = filterResourceType;

			const res = await auditLogs.list(params);
			items = res.items;
			total = res.total;
		} catch (e) {
			showError(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function applyFilters() {
		page = 1;
		load();
	}

	function clearFilters() {
		filterAction = '';
		filterResourceType = '';
		page = 1;
		load();
	}

	function goPage(p: number) {
		page = p;
		load();
	}

	const actionVariant = (action: string) => {
		if (action.includes('delete')) return 'error' as const;
		if (action.includes('create') || action.includes('register')) return 'success' as const;
		if (action.includes('update') || action.includes('activate') || action.includes('rollback')) return 'warning' as const;
		return 'outline' as const;
	};
</script>

<svelte:head>
	<title>Audit Log - hostedat</title>
</svelte:head>

<PageHeader title="Audit Log" description="Activity log of all actions performed on the platform" />

<!-- Filters -->
<div class="mb-4">
	<Button variant="ghost" size="sm" onclick={() => (showFilters = !showFilters)}>
		{#if showFilters}<X class="size-3.5" /> Hide filters{:else}<Filter class="size-3.5" /> Filters{/if}
	</Button>

	{#if showFilters}
		<div class="mt-2 flex flex-wrap gap-3 rounded-lg border border-border bg-base p-3">
			<div class="space-y-1">
				<label for="filter-action" class="text-xs font-medium text-text">Action</label>
				<select
					id="filter-action"
					bind:value={filterAction}
					class="block rounded-lg border border-border bg-base px-3 py-1.5 text-sm text-text focus:outline-none focus:ring-2 focus:ring-primary/50"
				>
					<option value="">All actions</option>
					{#each Object.entries(actionGroups) as [group, actions]}
						<optgroup label={group}>
							{#each actions as action}
								<option value={action}>{action}</option>
							{/each}
						</optgroup>
					{/each}
				</select>
			</div>

			<div class="space-y-1">
				<label for="filter-resource" class="text-xs font-medium text-text">Resource type</label>
				<select
					id="filter-resource"
					bind:value={filterResourceType}
					class="block rounded-lg border border-border bg-base px-3 py-1.5 text-sm text-text focus:outline-none focus:ring-2 focus:ring-primary/50"
				>
					<option value="">All types</option>
					{#each resourceTypes as rt}
						<option value={rt}>{rt}</option>
					{/each}
				</select>
			</div>

			<div class="flex items-end gap-2">
				<Button size="sm" onclick={applyFilters}>Apply</Button>
				<Button variant="ghost" size="sm" onclick={clearFilters}>Clear</Button>
			</div>
		</div>
	{/if}
</div>

<!-- Table -->
{#if loading}
	<div class="space-y-2">
		{#each Array(8) as _}
			<Skeleton class="h-12 rounded-lg" />
		{/each}
	</div>
{:else if items.length === 0}
	<div class="py-16 text-center">
		<p class="text-sm text-text-muted">No audit log entries found.</p>
	</div>
{:else}
	<div class="rounded-xl border border-border overflow-hidden">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border bg-elevated/50">
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted">Action</th>
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted hidden sm:table-cell">Actor</th>
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted hidden md:table-cell">Resource</th>
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted hidden lg:table-cell">IP</th>
					<th class="px-3 py-2.5 text-right text-xs font-medium text-text-muted">When</th>
				</tr>
			</thead>
			<tbody>
				{#each items as log (log.id)}
					<tr class="border-b border-border last:border-0 hover:bg-elevated/30 transition-colors">
						<td class="px-3 py-2.5">
							<Badge variant={actionVariant(log.action)}>{log.action}</Badge>
						</td>
						<td class="px-3 py-2.5 text-text-muted hidden sm:table-cell">
							<span class="truncate max-w-[180px] inline-block">{log.actor_email}</span>
						</td>
						<td class="px-3 py-2.5 hidden md:table-cell">
							<span class="text-text-muted">{log.resource_type}</span>
							{#if log.resource_id}
								<code class="text-xs font-mono text-text ml-1">{log.resource_id}</code>
							{/if}
						</td>
						<td class="px-3 py-2.5 text-text-muted font-mono text-xs hidden lg:table-cell">
							{log.ip_address}
						</td>
						<td class="px-3 py-2.5 text-right text-text-muted text-xs">
							{timeAgo(log.created_at)}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<!-- Pagination -->
	{#if totalPages > 1}
		<div class="flex items-center justify-between mt-4">
			<p class="text-xs text-text-muted">{total} entries</p>
			<div class="flex items-center gap-1">
				<Button variant="ghost" size="sm" onclick={() => goPage(page - 1)} disabled={page <= 1}>
					<ChevronLeft class="size-4" />
				</Button>
				<span class="text-xs text-text-muted px-2">{page} / {totalPages}</span>
				<Button variant="ghost" size="sm" onclick={() => goPage(page + 1)} disabled={page >= totalPages}>
					<ChevronRight class="size-4" />
				</Button>
			</div>
		</div>
	{/if}
{/if}
