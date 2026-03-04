<script lang="ts">
	import { admin } from '$api/client';
	import type { InstanceSettings } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Button from '$components/ui/Button.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	let settings = $state<InstanceSettings | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let message = $state('');

	async function load() {
		loading = true;
		try { settings = await admin.getSettings(); }
		catch { /* */ }
		finally { loading = false; }
	}

	onMount(load);

	async function handleSave() {
		if (!settings) return;
		saving = true;
		message = '';
		try {
			settings = await admin.updateSettings(settings);
			message = 'Settings saved.';
			setTimeout(() => (message = ''), 3000);
		} catch (err) {
			message = err instanceof Error ? err.message : 'Failed to save';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Settings - hostedat</title>
</svelte:head>

<PageHeader title="Instance Settings" description="Configure registration and access policies" />

{#if loading}
	<div class="space-y-3 max-w-md">
		<Skeleton class="h-10 rounded-lg" />
		<Skeleton class="h-10 rounded-lg" />
		<Skeleton class="h-10 rounded-lg w-28" />
	</div>
{:else if settings}
	<div class="space-y-6 max-w-md">
		<div class="space-y-4">
			<div class="flex items-center gap-3">
				<input
					type="checkbox"
					id="reg-enabled"
					bind:checked={settings.registration_enabled}
					class="rounded border-border accent-primary"
				/>
				<div>
					<label for="reg-enabled" class="text-sm font-medium text-text">Registration enabled</label>
					<p class="text-xs text-text-muted">Allow new users to create accounts</p>
				</div>
			</div>

			<div class="flex items-center gap-3">
				<input
					type="checkbox"
					id="invite-req"
					bind:checked={settings.invite_required}
					class="rounded border-border accent-primary"
				/>
				<div>
					<label for="invite-req" class="text-sm font-medium text-text">Invite required</label>
					<p class="text-xs text-text-muted">Require a valid invite code to register</p>
				</div>
			</div>
		</div>

		{#if message}
			<p class="text-sm text-primary">{message}</p>
		{/if}

		<Button onclick={handleSave} disabled={saving}>
			{#if saving}
				<Loader2 class="size-4 animate-spin" /> Saving...
			{:else}
				Save changes
			{/if}
		</Button>
	</div>
{/if}
