<script lang="ts">
	import type { DurableObjectNamespace } from '$api/types';
	import { workers } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Plus, Trash2, Loader2 } from 'lucide-svelte';

	interface Props { siteId: string; items: DurableObjectNamespace[]; onRefresh: () => void; }
	let { siteId, items, onRefresh }: Props = $props();

	let name = $state('');
	let adding = $state(false);
	let deletingId = $state<string | null>(null);

	async function handleAdd() {
		if (!name) return;
		adding = true;
		try { await workers.createDurableObject(siteId, name); name = ''; onRefresh(); }
		catch { /* */ } finally { adding = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try { await workers.deleteDurableObject(siteId, id); onRefresh(); }
		catch { /* */ } finally { deletingId = null; }
	}
</script>

<div class="space-y-4">
	<h4 class="font-semibold">Durable Objects</h4>
	<p class="text-xs text-text-muted">Stateful objects accessible via <code class="bg-elevated px-1 rounded">env.YOUR_DO</code></p>

	{#each items as ns (ns.id)}
		<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
			<div>
				<code class="text-sm font-mono text-text">{ns.name}</code>
				<p class="text-xs text-text-muted font-mono mt-0.5">{ns.namespace_id}</p>
			</div>
			<button onclick={() => handleDelete(ns.id)} class="text-text-muted hover:text-error p-1" disabled={deletingId === ns.id}>
				{#if deletingId === ns.id}<Loader2 class="size-3.5 animate-spin" />{:else}<Trash2 class="size-3.5" />{/if}
			</button>
		</div>
	{/each}

	<div class="flex gap-2">
		<Input bind:value={name} placeholder="Namespace name" class="font-mono text-xs" />
		<Button size="sm" onclick={handleAdd} disabled={adding || !name}>
			{#if adding}<Loader2 class="size-3.5 animate-spin" />{:else}<Plus class="size-3.5" />{/if}
		</Button>
	</div>
</div>
