<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		icon?: Snippet;
		title: string;
		description: string;
		action?: Snippet;
	}

	let { icon, title, description, action }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	{#if icon}
		<div class="mb-4 rounded-full bg-elevated p-4 text-text-muted">
			{@render icon()}
		</div>
	{/if}
	<h3 class="text-lg font-semibold text-text">{title}</h3>
	<p class="mt-1 text-sm text-text-muted max-w-xs leading-relaxed">{description}</p>
	{#if action}
		<div class="mt-4">
			{@render action()}
		</div>
	{/if}
</div>
