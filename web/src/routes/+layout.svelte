<script>
	import '../app.css';
	import { Toaster } from 'svelte-sonner';

	let { children } = $props();
</script>

<Toaster
	position="bottom-right"
	toastOptions={{
		style: 'background: var(--color-surface); color: var(--color-text); border: 1px solid var(--color-border);'
	}}
/>

{@render children()}
