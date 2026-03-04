<script lang="ts">
	import type { WorkerEnvVar } from '$api/types';
	import { workers } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Plus, Trash2, Loader2 } from 'lucide-svelte';
	import { showError } from '$lib/utils/errors';

	interface Props {
		siteId: string;
		items: WorkerEnvVar[];
		onRefresh: () => void;
	}

	let { siteId, items, onRefresh }: Props = $props();

	let name = $state('');
	let value = $state('');
	let secret = $state(false);
	let adding = $state(false);
	let deletingId = $state<string | null>(null);

	async function handleAdd() {
		if (!name || !value) return;
		adding = true;
		try {
			await workers.setEnv(siteId, { name, value, secret });
			name = ''; value = ''; secret = false;
			onRefresh();
		} catch (e) { showError(e); } finally { adding = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try {
			await workers.deleteEnv(siteId, id);
			onRefresh();
		} catch (e) { showError(e); } finally { deletingId = null; }
	}
</script>

<div class="space-y-4">
	<h4 class="font-semibold">Environment Variables</h4>

	{#if items.length > 0}
		<div class="space-y-2">
			{#each items as env (env.id)}
				<div class="flex items-center gap-2 rounded-lg border border-border bg-base p-3">
					<code class="text-xs font-mono text-text flex-1 truncate">{env.name}</code>
					{#if env.secret}
						<span class="text-xs text-text-muted">••••••</span>
					{:else}
						<code class="text-xs font-mono text-text-muted truncate max-w-[200px]">{env.value}</code>
					{/if}
					<button
						onclick={() => handleDelete(env.id)}
						class="text-text-muted hover:text-error transition-colors p-1"
						disabled={deletingId === env.id}
					>
						{#if deletingId === env.id}
							<Loader2 class="size-3.5 animate-spin" />
						{:else}
							<Trash2 class="size-3.5" />
						{/if}
					</button>
				</div>
			{/each}
		</div>
	{/if}

	<div class="flex gap-2 items-end">
		<div class="flex-1 space-y-1.5">
			<label for="env-name" class="text-xs font-medium text-text">Name</label>
			<Input id="env-name" bind:value={name} placeholder="MY_VAR" class="font-mono text-xs" />
		</div>
		<div class="flex-1 space-y-1.5">
			<label for="env-value" class="text-xs font-medium text-text">Value</label>
			<Input id="env-value" bind:value type={secret ? 'password' : 'text'} placeholder="value" class="text-xs" />
		</div>
		<label class="flex items-center gap-1.5 pb-2 text-xs text-text-muted">
			<input type="checkbox" bind:checked={secret} class="accent-primary" /> Secret
		</label>
		<Button size="sm" onclick={handleAdd} disabled={adding || !name || !value}>
			{#if adding}<Loader2 class="size-3.5 animate-spin" />{:else}<Plus class="size-3.5" />{/if}
		</Button>
	</div>
</div>
