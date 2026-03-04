<script lang="ts">
	import { goto } from '$app/navigation';
	import { user } from '$stores/auth';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import { Loader2 } from 'lucide-svelte';

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let error = $state('');

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await user.login(email, password);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Sign in - hostedat</title>
</svelte:head>

<div class="rounded-xl border border-border bg-surface/50 backdrop-blur-sm p-8">
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
				autocomplete="current-password"
			/>
		</div>

		{#if error}
			<p class="text-sm text-error">{error}</p>
		{/if}

		<Button type="submit" class="w-full" disabled={loading}>
			{#if loading}
				<Loader2 class="size-4 animate-spin" />
				Signing in...
			{:else}
				Sign in
			{/if}
		</Button>
	</form>

	<div class="mt-4 text-center">
		<span class="text-sm text-text-muted">
			No account?
			<a href="/register" class="text-text underline underline-offset-4 hover:text-primary transition-colors">
				Register
			</a>
		</span>
	</div>
</div>
