<script lang="ts">
	import { sidebarMobileOpen } from '$stores/sidebar';
	import { Menu } from 'lucide-svelte';

	function toggleMobile() {
		sidebarMobileOpen.update((v) => !v);
	}
</script>

<header class="md:hidden flex items-center h-14 px-4 border-b border-border bg-surface sticky top-0 z-30">
	<button
		onclick={toggleMobile}
		class="p-2 -ml-2 rounded-lg text-text-muted hover:text-text hover:bg-elevated transition-colors"
		aria-label="Open menu"
	>
		<Menu class="size-5" />
	</button>

	<a href="/" class="ml-3 font-mono font-bold text-text text-sm">
		<span class="text-text-muted">//</span>hostedat
	</a>
</header>
