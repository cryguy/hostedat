<script lang="ts">
	import { apiKeys, s3Credentials } from '$api/client';
	import type { APIKey, S3Credential } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Dialog from '$components/ui/Dialog.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { Plus, Trash2, Loader2, Copy, Check, Key, HardDrive } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { getInstanceDomain } from '$lib/utils/config';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	const domain = getInstanceDomain();
	const s3Endpoint = `https://storage.${domain}`;

	// --- API Keys ---
	let keys = $state<APIKey[]>([]);
	let keysLoading = $state(true);
	let showCreateKey = $state(false);
	let newKeyName = $state('');
	let creatingKey = $state(false);
	let deletingKeyId = $state<string | null>(null);
	let createdKey = $state<string | null>(null);
	let copiedKey = $state(false);

	async function loadKeys() {
		keysLoading = true;
		try { keys = await apiKeys.list(); }
		catch (e) { showError(e); }
		finally { keysLoading = false; }
	}

	async function handleCreateKey() {
		if (!newKeyName) return;
		creatingKey = true;
		try {
			const res = await apiKeys.create(newKeyName);
			createdKey = res.key;
			newKeyName = '';
			showCreateKey = false;
			loadKeys();
		} catch (e) { showError(e); }
		finally { creatingKey = false; }
	}

	async function handleDeleteKey(id: string) {
		deletingKeyId = id;
		try { await apiKeys.delete(id); loadKeys(); }
		catch (e) { showError(e); }
		finally { deletingKeyId = null; }
	}

	async function copyApiKey() {
		if (!createdKey) return;
		await navigator.clipboard.writeText(createdKey);
		copiedKey = true;
		setTimeout(() => (copiedKey = false), 2000);
	}

	// --- S3 Credentials ---
	let s3Creds = $state<S3Credential[]>([]);
	let s3Loading = $state(true);
	let showCreateS3 = $state(false);
	let newS3Name = $state('');
	let creatingS3 = $state(false);
	let deletingS3Id = $state<string | null>(null);
	let createdS3 = $state<{ access_key_id: string; secret_access_key: string } | null>(null);
	let copiedS3 = $state(false);

	async function loadS3() {
		s3Loading = true;
		try { s3Creds = await s3Credentials.list(); }
		catch (e) { showError(e); }
		finally { s3Loading = false; }
	}

	async function handleCreateS3() {
		if (!newS3Name) return;
		creatingS3 = true;
		try {
			const res = await s3Credentials.create(newS3Name);
			createdS3 = { access_key_id: res.access_key_id, secret_access_key: res.secret_access_key };
			newS3Name = '';
			showCreateS3 = false;
			loadS3();
		} catch (e) { showError(e); }
		finally { creatingS3 = false; }
	}

	async function handleDeleteS3(id: string) {
		deletingS3Id = id;
		try { await s3Credentials.delete(id); loadS3(); }
		catch (e) { showError(e); }
		finally { deletingS3Id = null; }
	}

	async function copyS3Cred() {
		if (!createdS3) return;
		await navigator.clipboard.writeText(`Access Key: ${createdS3.access_key_id}\nSecret Key: ${createdS3.secret_access_key}`);
		copiedS3 = true;
		setTimeout(() => (copiedS3 = false), 2000);
	}

	onMount(() => { loadKeys(); loadS3(); });
</script>

<svelte:head>
	<title>API Keys - hostedat</title>
</svelte:head>

<PageHeader title="Credentials" description="Manage API keys and S3 storage credentials" />

<!-- Newly created API key banner -->
{#if createdKey}
	<div class="mb-4 rounded-lg border border-primary/30 bg-primary/5 p-4">
		<p class="text-sm font-medium text-text mb-1">API key created</p>
		<p class="text-xs text-text-muted mb-2">Copy this key now — it won't be shown again.</p>
		<div class="flex items-center gap-2">
			<code class="flex-1 rounded bg-base border border-border px-3 py-1.5 text-xs font-mono text-text break-all">{createdKey}</code>
			<Button variant="secondary" size="sm" onclick={copyApiKey}>
				{#if copiedKey}<Check class="size-3.5" />{:else}<Copy class="size-3.5" />{/if}
			</Button>
		</div>
	</div>
{/if}

<!-- Newly created S3 credential banner -->
{#if createdS3}
	<div class="mb-4 rounded-lg border border-primary/30 bg-primary/5 p-4">
		<p class="text-sm font-medium text-text mb-1">S3 credential created</p>
		<p class="text-xs text-text-muted mb-2">Copy these credentials now — the secret key won't be shown again.</p>
		<div class="space-y-1.5">
			<div>
				<p class="text-xs text-text-muted">Access Key ID</p>
				<code class="text-xs font-mono text-text">{createdS3.access_key_id}</code>
			</div>
			<div>
				<p class="text-xs text-text-muted">Secret Access Key</p>
				<code class="text-xs font-mono text-text break-all">{createdS3.secret_access_key}</code>
			</div>
		</div>
		<Button variant="secondary" size="sm" class="mt-2" onclick={copyS3Cred}>
			{#if copiedS3}<Check class="size-3.5" /> Copied{:else}<Copy class="size-3.5" /> Copy both{/if}
		</Button>
	</div>
{/if}

<!-- API Keys section -->
<div class="mb-8">
	<div class="flex items-center justify-between mb-3">
		<div class="flex items-center gap-2">
			<Key class="size-4 text-text-muted" />
			<h2 class="text-lg font-semibold text-text">API Keys</h2>
		</div>
		<Button size="sm" onclick={() => (showCreateKey = true)}>
			<Plus class="size-3.5" /> Create key
		</Button>
	</div>

	{#if keysLoading}
		<div class="space-y-2">
			{#each Array(2) as _}
				<Skeleton class="h-14 rounded-lg" />
			{/each}
		</div>
	{:else if keys.length === 0}
		<p class="text-sm text-text-muted py-6 text-center">No API keys yet.</p>
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
						onclick={() => handleDeleteKey(key.id)}
						class="text-text-muted hover:text-error transition-colors p-1"
						disabled={deletingKeyId === key.id}
					>
						{#if deletingKeyId === key.id}
							<Loader2 class="size-4 animate-spin" />
						{:else}
							<Trash2 class="size-4" />
						{/if}
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- S3 Credentials section -->
<div>
	<div class="flex items-center justify-between mb-3">
		<div class="flex items-center gap-2">
			<HardDrive class="size-4 text-text-muted" />
			<div>
				<h2 class="text-lg font-semibold text-text">S3 Credentials</h2>
				<p class="text-xs text-text-muted">Endpoint: <code class="font-mono bg-elevated px-1.5 py-0.5 rounded">{s3Endpoint}</code></p>
			</div>
		</div>
		<Button size="sm" onclick={() => (showCreateS3 = true)}>
			<Plus class="size-3.5" /> Create credential
		</Button>
	</div>

	{#if s3Loading}
		<div class="space-y-2">
			{#each Array(2) as _}
				<Skeleton class="h-14 rounded-lg" />
			{/each}
		</div>
	{:else if s3Creds.length === 0}
		<p class="text-sm text-text-muted py-6 text-center">No S3 credentials yet.</p>
	{:else}
		<div class="space-y-2">
			{#each s3Creds as cred (cred.id)}
				<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
					<div>
						<p class="text-sm font-medium text-text">{cred.name}</p>
						<p class="text-xs text-text-muted font-mono mt-0.5">{cred.access_key_id}</p>
						<p class="text-xs text-text-muted">
							Created {timeAgo(cred.created_at)}
							{#if cred.last_used_at}
								&middot; Last used {timeAgo(cred.last_used_at)}
							{/if}
						</p>
					</div>
					<button
						onclick={() => handleDeleteS3(cred.id)}
						class="text-text-muted hover:text-error transition-colors p-1"
						disabled={deletingS3Id === cred.id}
					>
						{#if deletingS3Id === cred.id}
							<Loader2 class="size-4 animate-spin" />
						{:else}
							<Trash2 class="size-4" />
						{/if}
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create API Key dialog -->
<Dialog open={showCreateKey} onclose={() => (showCreateKey = false)} title="Create API Key">
	<form onsubmit={(e) => { e.preventDefault(); handleCreateKey(); }} class="space-y-4">
		<div class="space-y-1.5">
			<label for="key-name" class="text-sm font-medium text-text">Name</label>
			<Input id="key-name" bind:value={newKeyName} placeholder="e.g. CI/CD" />
		</div>
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showCreateKey = false)}>Cancel</Button>
			<Button type="submit" disabled={creatingKey || !newKeyName}>
				{#if creatingKey}<Loader2 class="size-4 animate-spin" />{:else}Create{/if}
			</Button>
		</div>
	</form>
</Dialog>

<!-- Create S3 Credential dialog -->
<Dialog open={showCreateS3} onclose={() => (showCreateS3 = false)} title="Create S3 Credential">
	<form onsubmit={(e) => { e.preventDefault(); handleCreateS3(); }} class="space-y-4">
		<div class="space-y-1.5">
			<label for="s3-name" class="text-sm font-medium text-text">Name</label>
			<Input id="s3-name" bind:value={newS3Name} placeholder="e.g. backup-bot" />
		</div>
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showCreateS3 = false)}>Cancel</Button>
			<Button type="submit" disabled={creatingS3 || !newS3Name}>
				{#if creatingS3}<Loader2 class="size-4 animate-spin" />{:else}Create{/if}
			</Button>
		</div>
	</form>
</Dialog>
