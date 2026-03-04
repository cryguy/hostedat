<script lang="ts">
	import { goto } from '$app/navigation';
	import { user } from '$stores/auth';

	let { children } = $props();

	// Redirect to dashboard if already logged in
	$effect(() => {
		if ($user) goto('/', { replaceState: true });
	});
</script>

<div class="relative min-h-screen flex items-center justify-center bg-base overflow-hidden">
	<!-- Dot grid background -->
	<div
		class="absolute inset-0 opacity-[0.04]"
		style="background-image: radial-gradient(circle, var(--text) 1px, transparent 1px); background-size: 24px 24px;"
	></div>

	<!-- Subtle green glow -->
	<div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-96 h-96 bg-primary/5 rounded-full blur-3xl"></div>

	<div class="relative w-full max-w-sm px-4">
		<!-- Logo -->
		<div class="mb-8 text-center">
			<div class="inline-flex items-center justify-center size-12 rounded-xl bg-primary/10 mb-3">
				<span class="text-primary font-mono font-bold text-lg">//</span>
			</div>
			<h1 class="text-xl font-mono font-bold tracking-tight text-text">hostedat</h1>
			<p class="mt-1 text-xs text-text-muted tracking-wide uppercase">Static site hosting</p>
		</div>

		{@render children()}
	</div>
</div>
