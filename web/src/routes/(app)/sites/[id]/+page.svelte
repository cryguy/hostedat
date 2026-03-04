<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { sites as sitesApi, deployments as deploymentsApi } from '$api/client';
	import type { Site, Deployment } from '$api/types';
	import Breadcrumb from '$components/shared/Breadcrumb.svelte';
	import StatusDot from '$components/ui/StatusDot.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import Tabs from '$components/ui/Tabs.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import DeployUpload from '$components/sites/DeployUpload.svelte';
	import DeployTimeline from '$components/sites/DeployTimeline.svelte';
	import SiteSettings from '$components/sites/SiteSettings.svelte';
	import WorkerPanel from '$components/sites/worker/WorkerPanel.svelte';
	import StoragePanel from '$components/sites/StoragePanel.svelte';
	import AnalyticsPanel from '$components/analytics/AnalyticsPanel.svelte';
	import { ExternalLink, Upload, History, BarChart3, Code, HardDrive, Settings } from 'lucide-svelte';
	import { getInstanceDomain } from '$lib/utils/config';

	const id = $page.params.id;
	const domain = getInstanceDomain();

	let site = $state<Site | null>(null);
	let deps = $state<Deployment[]>([]);
	let depsTotal = $state(0);
	let depsPage = $state(1);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let activeTab = $state('deploy');

	const tabs = [
		{ id: 'deploy', label: 'Deploy', icon: Upload },
		{ id: 'deployments', label: 'Versions', icon: History },
		{ id: 'analytics', label: 'Analytics', icon: BarChart3 },
		{ id: 'worker', label: 'Worker', icon: Code },
		{ id: 'storage', label: 'Storage', icon: HardDrive },
		{ id: 'settings', label: 'Settings', icon: Settings }
	];

	async function loadDeps(pg: number) {
		try {
			const d = await deploymentsApi.list(id, pg);
			deps = d.deployments;
			depsTotal = d.total;
			depsPage = pg;
		} catch { /* */ }
	}

	async function load() {
		error = null;
		try {
			const [s, d] = await Promise.all([
				sitesApi.get(id),
				deploymentsApi.list(id, depsPage)
			]);
			site = s;
			deps = d.deployments;
			depsTotal = d.total;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load site';
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{site?.name ?? 'Site'} - hostedat</title>
</svelte:head>

{#if loading}
	<div class="space-y-4">
		<Skeleton class="h-4 w-32" />
		<Skeleton class="h-8 w-64" />
		<Skeleton class="h-10 w-full" />
		<Skeleton class="h-64 w-full rounded-xl" />
	</div>
{:else if error || !site}
	<div class="flex flex-col items-center justify-center py-24 gap-4">
		<p class="text-sm text-text-muted">{error || 'Site not found'}</p>
		<a href="/" class="text-sm text-primary hover:underline">Back to sites</a>
	</div>
{:else}
	<Breadcrumb items={[{ label: 'Sites', href: '/' }, { label: site.name }]} />

	<!-- Site header -->
	<div class="flex items-center justify-between mb-6">
		<div>
			<div class="flex items-center gap-2">
				<StatusDot status={site.active_version !== null ? 'live' : 'empty'} size="md" />
				<h1 class="text-2xl font-bold tracking-tight text-text">{site.name}</h1>
			</div>
			<div class="flex items-center gap-2 mt-1">
				<a
					href="https://{site.subdomain_slug}.{domain}"
					target="_blank"
					rel="noopener noreferrer"
					class="text-sm text-text-muted font-mono hover:text-primary transition-colors inline-flex items-center gap-1"
				>
					{site.subdomain_slug}.{domain}
					<ExternalLink class="size-3" />
				</a>
				{#if site.spa_mode}
					<Badge variant="outline">SPA</Badge>
				{/if}
			</div>
		</div>
		{#if site.active_version !== null}
			<Badge variant="success">v{site.active_version}</Badge>
		{/if}
	</div>

	<!-- Tabs -->
	<Tabs {tabs} active={activeTab} onchange={(id) => (activeTab = id)}>
		{#if activeTab === 'deploy'}
			<DeployUpload siteId={site.id} onDeployed={load} />
		{:else if activeTab === 'deployments'}
			<DeployTimeline
				siteId={site.id}
				items={deps}
				activeVersion={site.active_version}
				page={depsPage}
				total={depsTotal}
				onRollback={load}
				onPageChange={loadDeps}
			/>
		{:else if activeTab === 'analytics'}
			<AnalyticsPanel siteId={site.id} />
		{:else if activeTab === 'worker'}
			<WorkerPanel siteId={site.id} hasWorker={site.has_worker} />
		{:else if activeTab === 'storage'}
			<StoragePanel siteId={site.id} />
		{:else if activeTab === 'settings'}
			<SiteSettings {site} onUpdated={load} />
		{/if}
	</Tabs>
{/if}
