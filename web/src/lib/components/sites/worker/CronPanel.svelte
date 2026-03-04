<script lang="ts">
	import type { CronSchedule } from '$api/types';
	import { workers } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import { Plus, Trash2, Loader2 } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';

	interface Props { siteId: string; items: CronSchedule[]; onRefresh: () => void; }
	let { siteId, items, onRefresh }: Props = $props();

	let cron = $state('');
	let adding = $state(false);
	let deletingId = $state<string | null>(null);

	async function handleAdd() {
		if (!cron) return;
		adding = true;
		try { await workers.createCron(siteId, { cron, enabled: true }); cron = ''; onRefresh(); }
		catch { /* */ } finally { adding = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try { await workers.deleteCron(siteId, id); onRefresh(); }
		catch { /* */ } finally { deletingId = null; }
	}
</script>

<div class="space-y-4">
	<h4 class="font-semibold">Cron Schedules</h4>
	<p class="text-xs text-text-muted">Periodic triggers for your worker</p>

	{#each items as schedule (schedule.id)}
		<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
			<div class="flex items-center gap-2">
				<code class="text-sm font-mono text-text">{schedule.cron}</code>
				<Badge variant={schedule.enabled ? 'success' : 'outline'}>
					{schedule.enabled ? 'Active' : 'Disabled'}
				</Badge>
			</div>
			<div class="flex items-center gap-2">
				{#if schedule.last_run_at}
					<span class="text-xs text-text-muted">Last: {timeAgo(schedule.last_run_at)}</span>
				{/if}
				<button onclick={() => handleDelete(schedule.id)} class="text-text-muted hover:text-error p-1" disabled={deletingId === schedule.id}>
					{#if deletingId === schedule.id}<Loader2 class="size-3.5 animate-spin" />{:else}<Trash2 class="size-3.5" />{/if}
				</button>
			</div>
		</div>
	{/each}

	<div class="flex gap-2">
		<Input bind:value={cron} placeholder="*/5 * * * *" class="font-mono text-xs" />
		<Button size="sm" onclick={handleAdd} disabled={adding || !cron}>
			{#if adding}<Loader2 class="size-3.5 animate-spin" />{:else}<Plus class="size-3.5" />{/if}
		</Button>
	</div>
</div>
