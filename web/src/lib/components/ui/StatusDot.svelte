<script lang="ts">
	interface Props {
		status: 'live' | 'deploying' | 'failed' | 'empty';
		size?: 'sm' | 'md';
	}

	let { status, size = 'sm' }: Props = $props();

	const colors: Record<string, string> = {
		live: 'bg-success',
		deploying: 'bg-warning',
		failed: 'bg-error',
		empty: 'bg-text-muted'
	};

	const sizes: Record<string, string> = {
		sm: 'size-2',
		md: 'size-2.5'
	};
</script>

<span class="relative inline-flex">
	{#if status === 'live'}
		<span class="absolute inline-flex h-full w-full animate-ping rounded-full {colors[status]} opacity-40"></span>
	{/if}
	<span
		class="relative inline-flex rounded-full {colors[status]} {sizes[size]}"
		class:animate-spin={status === 'deploying'}
	></span>
</span>
