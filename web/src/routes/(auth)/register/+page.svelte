<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { auth } from '$api/client';
	import { user } from '$stores/auth';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import { Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	// Pre-fill invite code from URL if present
	const urlInvite = $page.url.searchParams.get('invite') ?? '';

	let registrationEnabled = $state(true);
	let inviteRequired = $state(false);
	let settingsLoading = $state(true);

	let email = $state('');
	let password = $state('');
	let inviteCode = $state(urlInvite);
	let loading = $state(false);
	let error = $state('');

	onMount(async () => {
		try {
			const info = await auth.registrationInfo();
			registrationEnabled = info.registration_enabled;
			inviteRequired = info.invite_required;
		} catch {
			// If we can't fetch settings, assume defaults (enabled, no invite)
		} finally {
			settingsLoading = false;
		}
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await user.register(email, password, inviteCode || undefined);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Registration failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register - hostedat</title>
</svelte:head>

<div class="rounded-xl border border-border bg-surface/50 backdrop-blur-sm p-8">
	{#if settingsLoading}
		<div class="space-y-4">
			<Skeleton class="h-10 rounded-lg" />
			<Skeleton class="h-10 rounded-lg" />
			<Skeleton class="h-10 rounded-lg w-full" />
		</div>
	{:else if !registrationEnabled}
		<div class="text-center py-4">
			<p class="text-lg font-semibold text-text mb-2">Registration disabled</p>
			<p class="text-sm text-text-muted mb-4">This instance is not accepting new registrations.</p>
			<a href="/login" class="text-sm text-primary hover:underline">Sign in instead</a>
		</div>
	{:else}
		<form onsubmit={handleSubmit} class="space-y-4">
			<div class="space-y-1.5">
				<label for="email" class="text-sm font-medium text-text">Email</label>
				<Input
					id="email"
					type="email"
					placeholder="you@example.com"
					bind:value={email}
					required
					autocomplete="email"
					autofocus
				/>
			</div>

			<div class="space-y-1.5">
				<label for="password" class="text-sm font-medium text-text">Password</label>
				<Input
					id="password"
					type="password"
					bind:value={password}
					required
					autocomplete="new-password"
				/>
			</div>

			{#if inviteRequired || urlInvite}
				<div class="space-y-1.5">
					<label for="invite" class="text-sm font-medium text-text">
						Invite code
						{#if inviteRequired}<span class="text-error">*</span>{/if}
					</label>
					<Input
						id="invite"
						bind:value={inviteCode}
						placeholder="Enter invite code"
						class="font-mono text-xs"
						required={inviteRequired}
						disabled={!!urlInvite}
					/>
				</div>
			{/if}

			{#if error}
				<p class="text-sm text-error">{error}</p>
			{/if}

			<Button type="submit" class="w-full" disabled={loading}>
				{#if loading}
					<Loader2 class="size-4 animate-spin" />
					Creating account...
				{:else}
					Create account
				{/if}
			</Button>
		</form>

		<div class="mt-4 text-center">
			<span class="text-sm text-text-muted">
				Already have an account?
				<a href="/login" class="text-text underline underline-offset-4 hover:text-primary transition-colors">
					Sign in
				</a>
			</span>
		</div>
	{/if}
</div>
