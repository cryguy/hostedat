<script lang="ts">
	import { goto } from '$app/navigation';
	import { user } from '$stores/auth';
	import Sidebar from '$components/layout/Sidebar.svelte';
	import Topbar from '$components/layout/Topbar.svelte';

	let { children } = $props();

	// Redirect to login if not authenticated
	$effect(() => {
		if (!$user) goto('/login', { replaceState: true });
	});
</script>

{#if $user}
	<div class="flex min-h-screen bg-base">
		<Sidebar />
		<div class="flex-1 flex flex-col min-w-0">
			<Topbar />
			<main class="flex-1 p-6 max-w-6xl w-full mx-auto">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
