<script lang="ts">
	import type { D1Database } from '$api/types';
	import { workers } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Plus, Trash2, Loader2 } from 'lucide-svelte';
	import { showError } from '$lib/utils/errors';

	interface Props { siteId: string; items: D1Database[]; onRefresh: () => void; }
	let { siteId, items, onRefresh }: Props = $props();

	let name = $state('');
	let adding = $state(false);
	let deletingId = $state<string | null>(null);

	async function handleAdd() {
		if (!name) return;
		adding = true;
		try { await workers.createD1(siteId, name); name = ''; onRefresh(); }
		catch (e) { showError(e); } finally { adding = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try { await workers.deleteD1(siteId, id); onRefresh(); }
		catch (e) { showError(e); } finally { deletingId = null; }
	}
</script>

<div class="space-y-4">
	<h4 class="font-semibold">D1 Databases</h4>
	<p class="text-xs text-text-muted">SQLite databases accessible via <code class="bg-elevated px-1 rounded">env.YOUR_DB</code></p>

	{#each items as db (db.id)}
		<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
			<div>
				<code class="text-sm font-mono text-text">{db.name}</code>
				<p class="text-xs text-text-muted font-mono mt-0.5">{db.database_id}</p>
			</div>
			<button onclick={() => handleDelete(db.id)} class="text-text-muted hover:text-error p-1" disabled={deletingId === db.id}>
				{#if deletingId === db.id}<Loader2 class="size-3.5 animate-spin" />{:else}<Trash2 class="size-3.5" />{/if}
			</button>
		</div>
	{/each}

	<div class="flex gap-2">
		<Input bind:value={name} placeholder="Database name" class="font-mono text-xs" />
		<Button size="sm" onclick={handleAdd} disabled={adding || !name}>
			{#if adding}<Loader2 class="size-3.5 animate-spin" />{:else}<Plus class="size-3.5" />{/if}
		</Button>
	</div>
</div>
