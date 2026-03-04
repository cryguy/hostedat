<script lang="ts">
	import { apiKeys } from '$api/client';
	import type { APIKey } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Dialog from '$components/ui/Dialog.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { Plus, Trash2, Loader2, Copy, Check } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { onMount } from 'svelte';

	let keys = $state<APIKey[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let newName = $state('');
	let creating = $state(false);
	let deletingId = $state<string | null>(null);

	// Newly created key (shown once)
	let createdKey = $state<string | null>(null);
	let copied = $state(false);

	async function load() {
		loading = true;
		try { keys = await apiKeys.list(); }
		catch { /* */ }
		finally { loading = false; }
	}

	onMount(load);

	async function handleCreate() {
		if (!newName) return;
		creating = true;
		try {
			const res = await apiKeys.create(newName);
			createdKey = res.key;
			newName = '';
			showCreate = false;
			load();
		} catch { /* */ }
		finally { creating = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try { await apiKeys.delete(id); load(); }
		catch { /* */ }
		finally { deletingId = null; }
	}

	async function copyKey() {
		if (!createdKey) return;
		await navigator.clipboard.writeText(createdKey);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<svelte:head>
	<title>API Keys - hostedat</title>
</svelte:head>

<PageHeader title="API Keys" description="Manage API keys for CLI and programmatic access">
	<Button onclick={() => (showCreate = true)}>
		<Plus class="size-4" /> Create key
	</Button>
</PageHeader>

<!-- Newly created key banner -->
{#if createdKey}
	<div class="mb-4 rounded-lg border border-primary/30 bg-primary/5 p-4">
		<p class="text-sm font-medium text-text mb-1">Key created successfully</p>
		<p class="text-xs text-text-muted mb-2">Copy this key now — it won't be shown again.</p>
		<div class="flex items-center gap-2">
			<code class="flex-1 rounded bg-base border border-border px-3 py-1.5 text-xs font-mono text-text break-all">{createdKey}</code>
			<Button variant="secondary" size="sm" onclick={copyKey}>
				{#if copied}<Check class="size-3.5" />{:else}<Copy class="size-3.5" />{/if}
			</Button>
		</div>
	</div>
{/if}

{#if loading}
	<div class="space-y-2">
		{#each Array(3) as _}
			<Skeleton class="h-14 rounded-lg" />
		{/each}
	</div>
{:else if keys.length === 0}
	<div class="py-16 text-center">
		<p class="text-sm text-text-muted">No API keys yet. Create one to get started.</p>
	</div>
{:else}
	<div class="space-y-2">
		{#each keys as key (key.id)}
			<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
				<div>
					<p class="text-sm font-medium text-text">{key.name}</p>
					<p class="text-xs text-text-muted">
						Created {timeAgo(key.created_at)}
						{#if key.last_used_at}
							&middot; Last used {timeAgo(key.last_used_at)}
						{/if}
					</p>
				</div>
				<button
					onclick={() => handleDelete(key.id)}
					class="text-text-muted hover:text-error transition-colors p-1"
					disabled={deletingId === key.id}
				>
					{#if deletingId === key.id}
						<Loader2 class="size-4 animate-spin" />
					{:else}
						<Trash2 class="size-4" />
					{/if}
				</button>
			</div>
		{/each}
	</div>
{/if}

<!-- Create dialog -->
<Dialog open={showCreate} onclose={() => (showCreate = false)} title="Create API Key">
	<form onsubmit={(e) => { e.preventDefault(); handleCreate(); }} class="space-y-4">
		<div class="space-y-1.5">
			<label for="key-name" class="text-sm font-medium text-text">Name</label>
			<Input id="key-name" bind:value={newName} placeholder="e.g. CI/CD" />
		</div>
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showCreate = false)}>Cancel</Button>
			<Button type="submit" disabled={creating || !newName}>
				{#if creating}<Loader2 class="size-4 animate-spin" />{:else}Create{/if}
			</Button>
		</div>
	</form>
</Dialog>
