<script lang="ts">
	import type { Site } from '$api/types';
	import { sites } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Loader2, Trash2 } from 'lucide-svelte';
	import { goto } from '$app/navigation';

	interface Props {
		site: Site;
		onUpdated: () => void;
	}

	let { site, onUpdated }: Props = $props();

	let name = $state(site.name);
	let spaMode = $state(site.spa_mode);
	let saving = $state(false);
	let deleting = $state(false);
	let confirmDelete = $state('');
	let error = $state('');

	async function handleSave() {
		saving = true;
		error = '';
		try {
			await sites.update(site.id, { name, spa_mode: spaMode });
			onUpdated();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (confirmDelete !== site.subdomain_slug) return;
		deleting = true;
		try {
			await sites.delete(site.id);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete';
			deleting = false;
		}
	}
</script>

<div class="space-y-8 max-w-lg">
	<!-- General settings -->
	<div class="space-y-4">
		<h3 class="text-lg font-semibold">General</h3>

		<div class="space-y-1.5">
			<label for="site-name" class="text-sm font-medium text-text">Site name</label>
			<Input id="site-name" bind:value={name} />
		</div>

		<div class="flex items-center gap-3">
			<input
				type="checkbox"
				id="spa-mode"
				bind:checked={spaMode}
				class="rounded border-border accent-primary"
			/>
			<label for="spa-mode" class="text-sm text-text">SPA mode</label>
			<span class="text-xs text-text-muted">(Route all paths to index.html)</span>
		</div>

		{#if error}
			<p class="text-sm text-error">{error}</p>
		{/if}

		<Button onclick={handleSave} disabled={saving}>
			{#if saving}
				<Loader2 class="size-4 animate-spin" />
				Saving...
			{:else}
				Save changes
			{/if}
		</Button>
	</div>

	<!-- Danger zone -->
	<div class="rounded-xl border border-error/30 p-4 space-y-3">
		<h3 class="text-lg font-semibold text-error">Danger zone</h3>
		<p class="text-sm text-text-muted">
			Type <code class="font-mono text-xs bg-elevated px-1.5 py-0.5 rounded">{site.subdomain_slug}</code> to confirm deletion.
		</p>
		<Input
			placeholder={site.subdomain_slug}
			bind:value={confirmDelete}
			class="font-mono text-xs"
		/>
		<Button
			variant="danger"
			onclick={handleDelete}
			disabled={confirmDelete !== site.subdomain_slug || deleting}
		>
			{#if deleting}
				<Loader2 class="size-4 animate-spin" />
				Deleting...
			{:else}
				<Trash2 class="size-4" />
				Delete site
			{/if}
		</Button>
	</div>
</div>
