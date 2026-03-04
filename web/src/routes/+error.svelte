<script lang="ts">
	import { page } from '$app/stores';
	import Button from '$components/ui/Button.svelte';
</script>

<div class="min-h-screen flex flex-col items-center justify-center gap-4 bg-base">
	<span class="text-7xl font-mono font-bold text-text-muted/30">{$page.status}</span>
	<p class="text-text-muted">{$page.error?.message ?? 'Page not found'}</p>
	<Button variant="secondary" onclick={() => window.location.href = '/'}>Go home</Button>
</div>
