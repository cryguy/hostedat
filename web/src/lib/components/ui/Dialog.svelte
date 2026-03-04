<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fade, scale } from 'svelte/transition';

	interface Props {
		open: boolean;
		onClose: () => void;
		title: string;
		children: Snippet;
	}

	let { open, onClose, title, children }: Props = $props();

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onClose();
	}
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center"
		transition:fade={{ duration: 150 }}
	>
		<!-- Backdrop -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="absolute inset-0 bg-black/60 backdrop-blur-sm"
			onclick={onClose}
			onkeydown={(e) => e.key === 'Enter' && onClose()}
		></div>

		<!-- Content -->
		<div
			class="relative w-full max-w-md rounded-xl border border-border bg-surface p-6 shadow-2xl"
			transition:scale={{ start: 0.95, duration: 150 }}
			role="dialog"
			aria-modal="true"
			aria-label={title}
		>
			<h2 class="text-lg font-semibold text-text mb-4">{title}</h2>
			{@render children()}
		</div>
	</div>
{/if}
