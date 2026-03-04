<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	interface Props extends HTMLButtonAttributes {
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		size?: 'sm' | 'md' | 'lg';
		children: Snippet;
	}

	let { variant = 'primary', size = 'md', class: className = '', children, ...rest }: Props = $props();

	const base = 'inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-colors active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none cursor-pointer';

	const variants: Record<string, string> = {
		primary: 'bg-primary text-zinc-950 hover:bg-primary-hover',
		secondary: 'bg-surface border border-border text-text hover:bg-elevated',
		ghost: 'text-text-muted hover:text-text hover:bg-elevated',
		danger: 'bg-error/10 text-error hover:bg-error/20'
	};

	const sizes: Record<string, string> = {
		sm: 'h-8 px-3 text-xs',
		md: 'h-9 px-4 text-sm',
		lg: 'h-10 px-5 text-sm'
	};
</script>

<button class="{base} {variants[variant]} {sizes[size]} {className}" {...rest}>
	{@render children()}
</button>
