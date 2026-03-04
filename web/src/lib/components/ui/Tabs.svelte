<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Tab {
		id: string;
		label: string;
		icon?: any;
	}

	interface Props {
		tabs: Tab[];
		active: string;
		onchange: (id: string) => void;
		children: Snippet;
	}

	let { tabs, active, onchange, children }: Props = $props();
</script>

<div>
	<div class="flex gap-1 border-b border-border mb-6">
		{#each tabs as tab}
			<button
				onclick={() => onchange(tab.id)}
				class="relative px-4 py-2.5 text-sm font-medium transition-colors
					{active === tab.id
						? 'text-primary'
						: 'text-text-muted hover:text-text'}"
			>
				<span class="inline-flex items-center gap-2">
					{#if tab.icon}
						{@const Icon = tab.icon}
						<Icon class="size-4" />
					{/if}
					{tab.label}
				</span>
				{#if active === tab.id}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"></span>
				{/if}
			</button>
		{/each}
	</div>

	{@render children()}
</div>
