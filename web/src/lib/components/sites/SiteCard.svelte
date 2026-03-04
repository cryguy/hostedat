<script lang="ts">
	import type { Site } from '$api/types';
	import StatusDot from '$components/ui/StatusDot.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import { Code, ExternalLink } from 'lucide-svelte';
	import { timeAgo } from '$lib/utils/time';
	import { getInstanceDomain } from '$lib/utils/config';

	interface Props {
		site: Site;
	}

	let { site }: Props = $props();

	const domain = getInstanceDomain();
	let status = $derived(site.active_version !== null ? 'live' as const : 'empty' as const);
	let siteUrl = $derived(`https://${site.subdomain_slug}.${domain}`);
</script>

<a
	href="/sites/{site.id}"
	class="group block rounded-xl border border-border bg-surface p-4 transition-all hover:border-primary/30 hover:bg-elevated/50 hover:shadow-lg hover:shadow-primary/5"
>
	<div class="flex items-start justify-between mb-2">
		<div class="flex items-center gap-2 min-w-0">
			<StatusDot {status} />
			<span class="font-semibold text-text truncate">{site.name}</span>
		</div>
		{#if site.active_version !== null}
			<Badge variant="success">v{site.active_version}</Badge>
		{/if}
	</div>

	<div class="flex items-center gap-1 mb-3">
		<span class="text-xs font-mono text-text-muted truncate">{site.subdomain_slug}.{domain}</span>
		<ExternalLink class="size-3 text-text-muted shrink-0 opacity-0 group-hover:opacity-100 transition-opacity" />
	</div>

	<div class="flex items-center gap-3 text-xs text-text-muted">
		{#if site.has_worker}
			<span class="inline-flex items-center gap-1 text-info">
				<Code class="size-3" />
				Worker
			</span>
		{/if}
		{#if site.spa_mode}
			<Badge variant="outline">SPA</Badge>
		{/if}
		<span class="ml-auto">{timeAgo(site.created_at)}</span>
	</div>
</a>
