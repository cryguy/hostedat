<script lang="ts">
	import { onMount } from 'svelte';
	import { sites as sitesApi } from '$api/client';
	import type { Site } from '$api/types';
	import PageHeader from '$components/shared/PageHeader.svelte';
	import EmptyState from '$components/shared/EmptyState.svelte';
	import SiteCard from '$components/sites/SiteCard.svelte';
	import CreateSiteDialog from '$components/sites/CreateSiteDialog.svelte';
	import Skeleton from '$components/ui/Skeleton.svelte';
	import Button from '$components/ui/Button.svelte';
	import { Globe, Plus } from 'lucide-svelte';
	import { showError } from '$lib/utils/errors';

	let sitesList = $state<Site[]>([]);
	let loading = $state(true);
	let createOpen = $state(false);

	async function load() {
		try {
			sitesList = await sitesApi.list();
		} catch (e) {
			showError(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Sites - hostedat</title>
</svelte:head>

{#snippet action()}
	<Button onclick={() => (createOpen = true)}>
		<Plus class="size-4" />
		New site
	</Button>
{/snippet}

<PageHeader
	title="Sites"
	description="{sitesList.length} site{sitesList.length !== 1 ? 's' : ''}"
	{action}
/>

{#if loading}
	<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each Array(6) as _}
			<Skeleton class="h-32 rounded-xl" />
		{/each}
	</div>
{:else if sitesList.length === 0}
	{#snippet emptyIcon()}
		<Globe class="size-10" />
	{/snippet}
	{#snippet emptyAction()}
		<Button onclick={() => (createOpen = true)}>
			<Plus class="size-4" />
			New site
		</Button>
	{/snippet}
	<EmptyState
		icon={emptyIcon}
		title="No sites yet"
		description="Create your first site to get started with static hosting."
		action={emptyAction}
	/>
{:else}
	<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each sitesList as site (site.id)}
			<SiteCard {site} />
		{/each}
	</div>
{/if}

<CreateSiteDialog
	open={createOpen}
	onClose={() => (createOpen = false)}
	onCreated={load}
/>
