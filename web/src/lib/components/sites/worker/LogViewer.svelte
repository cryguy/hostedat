<script lang="ts">
	import type { WorkerLog } from '$api/types';
	import Button from '$components/ui/Button.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import { RefreshCw } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';

	interface Props { items: WorkerLog[]; onRefresh: () => void; }
	let { items, onRefresh }: Props = $props();

	const levelVariant = (level: string) => {
		if (level === 'error') return 'error' as const;
		if (level === 'warn') return 'warning' as const;
		return 'outline' as const;
	};
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h4 class="font-semibold">Worker Logs</h4>
		<Button variant="ghost" size="sm" onclick={onRefresh}>
			<RefreshCw class="size-3.5" /> Refresh
		</Button>
	</div>

	{#if items.length === 0}
		<p class="text-sm text-text-muted py-4 text-center">No logs yet.</p>
	{:else}
		<div class="space-y-1 max-h-96 overflow-y-auto rounded-lg border border-border bg-base p-2">
			{#each items as log (log.id)}
				<div class="flex items-start gap-2 px-2 py-1.5 text-xs rounded hover:bg-elevated">
					<Badge variant={levelVariant(log.level)}>{log.level}</Badge>
					<code class="font-mono text-text-muted flex-1 break-all">{log.message}</code>
					<span class="text-text-muted shrink-0">{timeAgo(log.created_at)}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
