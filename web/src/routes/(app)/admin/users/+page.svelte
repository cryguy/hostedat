<script lang="ts">
	import { admin } from '$api/client';
	import type { User } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import Button from '$components/ui/Button.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { ChevronLeft, ChevronRight, Loader2, Trash2 } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	let users = $state<User[]>([]);
	let total = $state(0);
	let page = $state(1);
	let loading = $state(true);
	let updatingId = $state<string | null>(null);
	let deletingId = $state<string | null>(null);

	const perPage = 50;
	const totalPages = $derived(Math.ceil(total / perPage));

	async function load() {
		loading = true;
		try {
			const res = await admin.listUsers(page);
			users = res.users;
			total = res.total;
		} catch (e) { showError(e); }
		finally { loading = false; }
	}

	onMount(load);

	async function changeRole(user: User, role: string) {
		updatingId = user.id;
		try { await admin.updateUserRole(user.id, role); load(); }
		catch (e) { showError(e); }
		finally { updatingId = null; }
	}

	async function deleteUser(id: string) {
		if (!confirm('Are you sure you want to delete this user?')) return;
		deletingId = id;
		try { await admin.deleteUser(id); load(); }
		catch (e) { showError(e); }
		finally { deletingId = null; }
	}

	function goPage(p: number) { page = p; load(); }

	const roleVariant = (role: string) => {
		if (role === 'superadmin') return 'error' as const;
		if (role === 'admin') return 'warning' as const;
		return 'outline' as const;
	};
</script>

<svelte:head>
	<title>Users - hostedat</title>
</svelte:head>

<PageHeader title="Users" description="Manage registered users and their roles" />

{#if loading}
	<div class="space-y-2">
		{#each Array(5) as _}
			<Skeleton class="h-14 rounded-lg" />
		{/each}
	</div>
{:else if users.length === 0}
	<div class="py-16 text-center">
		<p class="text-sm text-text-muted">No users found.</p>
	</div>
{:else}
	<div class="rounded-xl border border-border overflow-hidden">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-border bg-elevated/50">
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted">Email</th>
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted">Role</th>
					<th class="px-3 py-2.5 text-left text-xs font-medium text-text-muted hidden sm:table-cell">Joined</th>
					<th class="px-3 py-2.5 text-right text-xs font-medium text-text-muted">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each users as user (user.id)}
					<tr class="border-b border-border last:border-0 hover:bg-elevated/30 transition-colors">
						<td class="px-3 py-2.5 text-text">{user.email}</td>
						<td class="px-3 py-2.5">
							<Badge variant={roleVariant(user.role)}>{user.role}</Badge>
						</td>
						<td class="px-3 py-2.5 text-text-muted text-xs hidden sm:table-cell">
							{timeAgo(user.created_at)}
						</td>
						<td class="px-3 py-2.5 text-right">
							<div class="flex items-center justify-end gap-1">
								{#if user.role !== 'superadmin'}
									<select
										value={user.role}
										onchange={(e) => changeRole(user, (e.target as HTMLSelectElement).value)}
										disabled={updatingId === user.id}
										class="rounded border border-border bg-base px-2 py-1 text-xs text-text"
									>
										<option value="user">user</option>
										<option value="admin">admin</option>
									</select>
									<button
										onclick={() => deleteUser(user.id)}
										class="text-text-muted hover:text-error transition-colors p-1"
										disabled={deletingId === user.id}
									>
										{#if deletingId === user.id}
											<Loader2 class="size-3.5 animate-spin" />
										{:else}
											<Trash2 class="size-3.5" />
										{/if}
									</button>
								{/if}
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	{#if totalPages > 1}
		<div class="flex items-center justify-between mt-4">
			<p class="text-xs text-text-muted">{total} users</p>
			<div class="flex items-center gap-1">
				<Button variant="ghost" size="sm" onclick={() => goPage(page - 1)} disabled={page <= 1}>
					<ChevronLeft class="size-4" />
				</Button>
				<span class="text-xs text-text-muted px-2">{page} / {totalPages}</span>
				<Button variant="ghost" size="sm" onclick={() => goPage(page + 1)} disabled={page >= totalPages}>
					<ChevronRight class="size-4" />
				</Button>
			</div>
		</div>
	{/if}
{/if}
