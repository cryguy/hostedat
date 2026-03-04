<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		description?: string;
		action?: Snippet;
	}

	let { title, description, action }: Props = $props();
</script>

<div class="flex items-center justify-between pb-4 border-b border-border mb-6">
	<div>
		<h1 class="text-2xl font-bold tracking-tight text-text">{title}</h1>
		{#if description}
			<p class="text-sm text-text-muted mt-0.5">{description}</p>
		{/if}
	</div>
	{#if action}
		{@render action()}
	{/if}
</div>
