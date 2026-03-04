<script lang="ts">
	import { admin } from '$api/client';
	import type { Invite } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import Dialog from '$components/ui/Dialog.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { Plus, Trash2, Loader2, Copy, Check } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	let invites = $state<Invite[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let maxUses = $state('');
	let creating = $state(false);
	let deletingId = $state<string | null>(null);
	let copiedId = $state<string | null>(null);

	async function load() {
		loading = true;
		try { invites = await admin.listInvites(); }
		catch (e) { showError(e); }
		finally { loading = false; }
	}

	onMount(load);

	async function handleCreate() {
		creating = true;
		try {
			const data: { max_uses?: number } = {};
			if (maxUses) data.max_uses = parseInt(maxUses);
			await admin.createInvite(data);
			maxUses = '';
			showCreate = false;
			load();
		} catch (e) { showError(e); }
		finally { creating = false; }
	}

	async function handleRevoke(id: string) {
		deletingId = id;
		try { await admin.revokeInvite(id); load(); }
		catch (e) { showError(e); }
		finally { deletingId = null; }
	}

	async function copyCode(code: string, id: string) {
		await navigator.clipboard.writeText(code);
		copiedId = id;
		setTimeout(() => (copiedId = null), 2000);
	}
</script>

<svelte:head>
	<title>Invites - hostedat</title>
</svelte:head>

<PageHeader title="Invite Codes" description="Generate invite codes for new user registration">
	<Button onclick={() => (showCreate = true)}>
		<Plus class="size-4" /> Create invite
	</Button>
</PageHeader>

{#if loading}
	<div class="space-y-2">
		{#each Array(3) as _}
			<Skeleton class="h-14 rounded-lg" />
		{/each}
	</div>
{:else if invites.length === 0}
	<div class="py-16 text-center">
		<p class="text-sm text-text-muted">No invite codes yet.</p>
	</div>
{:else}
	<div class="space-y-2">
		{#each invites as invite (invite.id)}
			<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
				<div class="flex items-center gap-3 min-w-0">
					<div class="min-w-0">
						<div class="flex items-center gap-2">
							<code class="text-sm font-mono text-text">{invite.code}</code>
							<button onclick={() => copyCode(invite.code, invite.id)} class="text-text-muted hover:text-text p-0.5">
								{#if copiedId === invite.id}<Check class="size-3" />{:else}<Copy class="size-3" />{/if}
							</button>
							<Badge variant={invite.active ? 'success' : 'outline'}>
								{invite.active ? 'Active' : 'Revoked'}
							</Badge>
						</div>
						<p class="text-xs text-text-muted mt-0.5">
							Used {invite.use_count}{invite.max_uses ? `/${invite.max_uses}` : ''} times
							&middot; Created {timeAgo(invite.created_at)}
							{#if invite.expires_at}
								&middot; Expires {timeAgo(invite.expires_at)}
							{/if}
						</p>
					</div>
				</div>
				{#if invite.active}
					<button
						onclick={() => handleRevoke(invite.id)}
						class="text-text-muted hover:text-error transition-colors p-1"
						disabled={deletingId === invite.id}
					>
						{#if deletingId === invite.id}
							<Loader2 class="size-4 animate-spin" />
						{:else}
							<Trash2 class="size-4" />
						{/if}
					</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<!-- Create dialog -->
<Dialog open={showCreate} onclose={() => (showCreate = false)} title="Create Invite Code">
	<form onsubmit={(e) => { e.preventDefault(); handleCreate(); }} class="space-y-4">
		<div class="space-y-1.5">
			<label for="max-uses" class="text-sm font-medium text-text">Max uses (optional)</label>
			<Input id="max-uses" bind:value={maxUses} placeholder="Unlimited" type="number" />
		</div>
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showCreate = false)}>Cancel</Button>
			<Button type="submit" disabled={creating}>
				{#if creating}<Loader2 class="size-4 animate-spin" />{:else}Create{/if}
			</Button>
		</div>
	</form>
</Dialog>
