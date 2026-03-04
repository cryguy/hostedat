<script lang="ts">
	import { sites } from '$api/client';
	import Dialog from '$components/ui/Dialog.svelte';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Loader2 } from 'lucide-svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		onCreated: () => void;
	}

	let { open, onClose, onCreated }: Props = $props();

	let name = $state('');
	let slug = $state('');
	let loading = $state(false);
	let error = $state('');

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await sites.create(name, slug || undefined);
			name = '';
			slug = '';
			onCreated();
			onClose();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create site';
		} finally {
			loading = false;
		}
	}
</script>

<Dialog {open} {onClose} title="Create new site">
	<form onsubmit={handleSubmit} class="space-y-4">
		<div class="space-y-1.5">
			<label for="site-name" class="text-sm font-medium text-text">Name</label>
			<Input
				id="site-name"
				placeholder="My Site"
				bind:value={name}
				required
				autofocus
			/>
		</div>

		<div class="space-y-1.5">
			<label for="site-slug" class="text-sm font-medium text-text">Subdomain</label>
			<Input
				id="site-slug"
				placeholder="my-site (optional, auto-generated)"
				bind:value={slug}
				class="font-mono text-xs"
			/>
			<p class="text-xs text-text-muted">Leave blank to auto-generate from name</p>
		</div>

		{#if error}
			<p class="text-sm text-error">{error}</p>
		{/if}

		<div class="flex justify-end gap-2 pt-2">
			<Button variant="ghost" type="button" onclick={onClose}>Cancel</Button>
			<Button type="submit" disabled={loading}>
				{#if loading}
					<Loader2 class="size-4 animate-spin" />
					Creating...
				{:else}
					Create site
				{/if}
			</Button>
		</div>
	</form>
</Dialog>
