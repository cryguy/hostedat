<script lang="ts">
	import type { StorageBucket } from '$api/types';
	import { storage } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import Input from '$components/ui/Input.svelte';
	import Badge from '$components/ui/Badge.svelte';
	import { Plus, Trash2, Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { showError } from '$lib/utils/errors';

	interface Props { siteId: string; }
	let { siteId }: Props = $props();

	let buckets = $state<StorageBucket[]>([]);
	let name = $state('');
	let bucketSuffix = $state('');
	let isPublic = $state(false);
	let adding = $state(false);
	let deletingId = $state<string | null>(null);

	const bucketPrefix = `${siteId}-`;
	const fullBucketName = $derived(`${bucketPrefix}${bucketSuffix}`);

	async function load() {
		buckets = await storage.listBuckets(siteId);
	}

	onMount(load);

	async function handleAdd() {
		if (!name || !bucketSuffix) return;
		adding = true;
		try {
			await storage.createBucket(siteId, { name, bucket_name: fullBucketName, public: isPublic });
			name = ''; bucketSuffix = ''; isPublic = false;
			load();
		} catch (e) { showError(e); } finally { adding = false; }
	}

	async function handleDelete(id: string) {
		deletingId = id;
		try { await storage.deleteBucket(siteId, id); load(); }
		catch (e) { showError(e); } finally { deletingId = null; }
	}

	async function togglePublic(bucket: StorageBucket) {
		try {
			await storage.updateBucket(siteId, bucket.id, { public: !bucket.public });
			load();
		} catch (e) { showError(e); }
	}
</script>

<div class="space-y-4">
	<div>
		<h3 class="text-lg font-semibold mb-1">Storage Buckets</h3>
		<p class="text-sm text-text-muted">
			S3-compatible object storage. Workers access these via <code class="bg-elevated px-1.5 py-0.5 rounded text-xs">env.YOUR_BUCKET</code>.
		</p>
	</div>

	{#each buckets as bucket (bucket.id)}
		<div class="flex items-center justify-between rounded-lg border border-border bg-base p-3">
			<div>
				<div class="flex items-center gap-2">
					<code class="text-sm font-mono text-text">{bucket.name}</code>
					<Badge variant={bucket.public ? 'warning' : 'outline'}>
						{bucket.public ? 'Public' : 'Private'}
					</Badge>
				</div>
				<p class="text-xs text-text-muted font-mono mt-0.5">{bucket.bucket_name}</p>
			</div>
			<div class="flex items-center gap-2">
				<Button variant="ghost" size="sm" onclick={() => togglePublic(bucket)}>
					Make {bucket.public ? 'private' : 'public'}
				</Button>
				<button onclick={() => handleDelete(bucket.id)} class="text-text-muted hover:text-error p-1" disabled={deletingId === bucket.id}>
					{#if deletingId === bucket.id}<Loader2 class="size-3.5 animate-spin" />{:else}<Trash2 class="size-3.5" />{/if}
				</button>
			</div>
		</div>
	{/each}

	<div class="space-y-2 rounded-lg border border-border p-4">
		<div class="flex gap-2">
			<div class="flex-1 space-y-1">
				<label for="bucket-binding" class="text-xs font-medium text-text">Binding name</label>
				<Input id="bucket-binding" bind:value={name} placeholder="MY_BUCKET" class="font-mono text-xs" />
			</div>
			<div class="flex-1 space-y-1">
				<label for="bucket-name" class="text-xs font-medium text-text">Bucket name</label>
				<div class="flex items-center rounded-lg border border-border bg-surface focus-within:ring-2 focus-within:ring-primary/40 focus-within:border-primary transition-colors">
					<span class="px-2.5 text-xs font-mono text-text-muted select-none shrink-0">{bucketPrefix}</span>
					<input
						id="bucket-name"
						bind:value={bucketSuffix}
						placeholder="my-bucket"
						class="h-9 w-full bg-transparent pr-3 text-xs font-mono text-text placeholder:text-text-muted focus:outline-none"
					/>
				</div>
			</div>
		</div>
		<div class="flex items-center justify-between">
			<label class="flex items-center gap-1.5 text-xs text-text-muted">
				<input type="checkbox" bind:checked={isPublic} class="accent-primary" /> Public access
			</label>
			<Button size="sm" onclick={handleAdd} disabled={adding || !name || !bucketSuffix}>
				{#if adding}<Loader2 class="size-3.5 animate-spin" />{:else}<Plus class="size-3.5" /> Create bucket{/if}
			</Button>
		</div>
	</div>
</div>
