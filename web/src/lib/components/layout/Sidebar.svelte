<script lang="ts">
	import { page } from '$app/stores';
	import { sidebarCollapsed, sidebarMobileOpen } from '$stores/sidebar';
	import { user, isAdmin } from '$stores/auth';
	import { theme } from '$stores/theme';
	import {
		Globe,
		Key,
		ScrollText,
		Users,
		Settings,
		Mail,
		Sun,
		Moon,
		ChevronsLeft,
		ChevronsRight,
		LogOut
	} from 'lucide-svelte';

	const mainNav = [
		{ href: '/', label: 'Sites', icon: Globe },
		{ href: '/keys', label: 'API Keys', icon: Key },
		{ href: '/audit', label: 'Audit Log', icon: ScrollText }
	];

	const adminNav = [
		{ href: '/admin/users', label: 'Users', icon: Users },
		{ href: '/admin/settings', label: 'Settings', icon: Settings },
		{ href: '/admin/invites', label: 'Invites', icon: Mail }
	];

	function isActive(href: string, pathname: string): boolean {
		if (href === '/') return pathname === '/';
		return pathname.startsWith(href);
	}

	function toggleTheme() {
		theme.update((t) => (t === 'dark' ? 'light' : 'dark'));
	}

	function toggleCollapse() {
		sidebarCollapsed.update((c) => !c);
	}

	function handleLogout() {
		user.logout();
	}
</script>

<!-- Desktop sidebar -->
<aside
	class="hidden md:flex flex-col border-r border-border bg-surface transition-[width] duration-200 h-screen sticky top-0"
	style:width={$sidebarCollapsed ? '64px' : '240px'}
>
	<!-- Logo -->
	<div class="flex items-center h-14 px-4 border-b border-border shrink-0">
		<a href="/" class="font-mono font-bold text-text text-sm truncate">
			<span class="text-text-muted">//</span>{$sidebarCollapsed ? 'h' : 'hostedat'}
		</a>
	</div>

	<!-- Nav -->
	<nav class="flex-1 py-3 px-2 space-y-1 overflow-y-auto">
		{#each mainNav as item}
			{@const active = isActive(item.href, $page.url.pathname)}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-lg px-3 h-9 text-sm transition-colors
					{active
						? 'bg-primary/10 text-primary border-l-2 border-primary'
						: 'text-text-muted hover:text-text hover:bg-elevated'}"
				title={$sidebarCollapsed ? item.label : undefined}
			>
				<item.icon class="size-4 shrink-0" />
				{#if !$sidebarCollapsed}
					<span class="truncate">{item.label}</span>
				{/if}
			</a>
		{/each}

		{#if $isAdmin}
			<div class="pt-4 pb-1 px-3">
				{#if !$sidebarCollapsed}
					<span class="text-[10px] font-semibold uppercase tracking-wider text-text-muted">Admin</span>
				{:else}
					<div class="border-t border-border"></div>
				{/if}
			</div>
			{#each adminNav as item}
				{@const active = isActive(item.href, $page.url.pathname)}
				<a
					href={item.href}
					class="flex items-center gap-3 rounded-lg px-3 h-9 text-sm transition-colors
						{active
							? 'bg-primary/10 text-primary border-l-2 border-primary'
							: 'text-text-muted hover:text-text hover:bg-elevated'}"
					title={$sidebarCollapsed ? item.label : undefined}
				>
					<item.icon class="size-4 shrink-0" />
					{#if !$sidebarCollapsed}
						<span class="truncate">{item.label}</span>
					{/if}
				</a>
			{/each}
		{/if}
	</nav>

	<!-- Footer -->
	<div class="border-t border-border p-2 space-y-1 shrink-0">
		<button
			onclick={toggleTheme}
			class="flex items-center gap-3 rounded-lg px-3 h-9 w-full text-sm text-text-muted hover:text-text hover:bg-elevated transition-colors"
			title={$sidebarCollapsed ? 'Toggle theme' : undefined}
		>
			{#if $theme === 'dark'}
				<Sun class="size-4 shrink-0" />
			{:else}
				<Moon class="size-4 shrink-0" />
			{/if}
			{#if !$sidebarCollapsed}
				<span class="truncate">Toggle theme</span>
			{/if}
		</button>

		<button
			onclick={toggleCollapse}
			class="flex items-center gap-3 rounded-lg px-3 h-9 w-full text-sm text-text-muted hover:text-text hover:bg-elevated transition-colors"
		>
			{#if $sidebarCollapsed}
				<ChevronsRight class="size-4 shrink-0" />
			{:else}
				<ChevronsLeft class="size-4 shrink-0" />
				<span class="truncate">Collapse</span>
			{/if}
		</button>

		{#if $user}
			<button
				onclick={handleLogout}
				class="flex items-center gap-3 rounded-lg px-3 h-9 w-full text-sm text-text-muted hover:text-text hover:bg-elevated transition-colors"
				title={$sidebarCollapsed ? 'Logout' : undefined}
			>
				<LogOut class="size-4 shrink-0" />
				{#if !$sidebarCollapsed}
					<span class="truncate">{$user.email}</span>
				{/if}
			</button>
		{/if}
	</div>
</aside>

<!-- Mobile overlay -->
{#if $sidebarMobileOpen}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="fixed inset-0 z-40 md:hidden">
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="absolute inset-0 bg-black/50 backdrop-blur-sm"
			onclick={() => sidebarMobileOpen.set(false)}
			onkeydown={(e) => e.key === 'Escape' && sidebarMobileOpen.set(false)}
		></div>

		<aside class="absolute inset-y-0 left-0 w-64 bg-surface border-r border-border flex flex-col">
			<div class="flex items-center h-14 px-4 border-b border-border">
				<a href="/" class="font-mono font-bold text-text text-sm" onclick={() => sidebarMobileOpen.set(false)}>
					<span class="text-text-muted">//</span>hostedat
				</a>
			</div>

			<nav class="flex-1 py-3 px-2 space-y-1">
				{#each mainNav as item}
					{@const active = isActive(item.href, $page.url.pathname)}
					<a
						href={item.href}
						onclick={() => sidebarMobileOpen.set(false)}
						class="flex items-center gap-3 rounded-lg px-3 h-9 text-sm transition-colors
							{active
								? 'bg-primary/10 text-primary border-l-2 border-primary'
								: 'text-text-muted hover:text-text hover:bg-elevated'}"
					>
						<item.icon class="size-4 shrink-0" />
						<span>{item.label}</span>
					</a>
				{/each}

				{#if $isAdmin}
					<div class="pt-4 pb-1 px-3">
						<span class="text-[10px] font-semibold uppercase tracking-wider text-text-muted">Admin</span>
					</div>
					{#each adminNav as item}
						{@const active = isActive(item.href, $page.url.pathname)}
						<a
							href={item.href}
							onclick={() => sidebarMobileOpen.set(false)}
							class="flex items-center gap-3 rounded-lg px-3 h-9 text-sm transition-colors
								{active
									? 'bg-primary/10 text-primary border-l-2 border-primary'
									: 'text-text-muted hover:text-text hover:bg-elevated'}"
						>
							<item.icon class="size-4 shrink-0" />
							<span>{item.label}</span>
						</a>
					{/each}
				{/if}
			</nav>

			<div class="border-t border-border p-2 shrink-0">
				<button
					onclick={toggleTheme}
					class="flex items-center gap-3 rounded-lg px-3 h-9 w-full text-sm text-text-muted hover:text-text hover:bg-elevated transition-colors"
				>
					{#if $theme === 'dark'}
						<Sun class="size-4 shrink-0" />
					{:else}
						<Moon class="size-4 shrink-0" />
					{/if}
					<span>Toggle theme</span>
				</button>

				{#if $user}
					<button
						onclick={handleLogout}
						class="flex items-center gap-3 rounded-lg px-3 h-9 w-full text-sm text-text-muted hover:text-text hover:bg-elevated transition-colors"
					>
						<LogOut class="size-4 shrink-0" />
						<span class="truncate">{$user.email}</span>
					</button>
				{/if}
			</div>
		</aside>
	</div>
{/if}
