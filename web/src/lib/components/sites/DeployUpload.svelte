<script lang="ts">
	import { deployments } from '$api/client';
	import Button from '$components/ui/Button.svelte';
	import { Upload, Loader2 } from 'lucide-svelte';

	interface Props {
		siteId: string;
		onDeployed: () => void;
	}

	let { siteId, onDeployed }: Props = $props();

	let file = $state<File | null>(null);
	let loading = $state(false);
	let error = $state('');
	let dragOver = $state(false);

	function handleFile(f: File) {
		if (!f.name.endsWith('.tar.gz') && !f.name.endsWith('.tgz') && !f.name.endsWith('.zip')) {
			error = 'File must be a .tar.gz or .zip archive';
			return;
		}
		file = f;
		error = '';
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		const f = e.dataTransfer?.files[0];
		if (f) handleFile(f);
	}

	function handleInput(e: Event) {
		const input = e.target as HTMLInputElement;
		const f = input.files?.[0];
		if (f) handleFile(f);
	}

	async function deploy() {
		if (!file) return;
		loading = true;
		error = '';
		try {
			await deployments.deploy(siteId, file);
			file = null;
			onDeployed();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Deploy failed';
		} finally {
			loading = false;
		}
	}
</script>

<div class="space-y-4">
	<div>
		<h3 class="text-lg font-semibold mb-1">Deploy</h3>
		<p class="text-sm text-text-muted">Upload a .tar.gz or .zip archive of your site.</p>
	</div>

	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="relative rounded-xl border-2 border-dashed p-8 text-center transition-colors
			{dragOver ? 'border-primary bg-primary/5' : 'border-border hover:border-text-muted'}"
		ondragover={(e) => { e.preventDefault(); dragOver = true; }}
		ondragleave={() => (dragOver = false)}
		ondrop={handleDrop}
	>
		<input
			type="file"
			accept=".tar.gz,.tgz,.zip"
			class="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
			onchange={handleInput}
		/>
		<Upload class="size-8 mx-auto mb-2 text-text-muted" />
		{#if file}
			<p class="text-sm font-medium text-text">{file.name}</p>
			<p class="text-xs text-text-muted mt-1">{(file.size / 1024).toFixed(1)} KB</p>
		{:else}
			<p class="text-sm text-text-muted">Drop your archive here or click to browse</p>
		{/if}
	</div>

	{#if error}
		<p class="text-sm text-error">{error}</p>
	{/if}

	{#if file}
		<Button onclick={deploy} disabled={loading} class="w-full">
			{#if loading}
				<Loader2 class="size-4 animate-spin" />
				Deploying...
			{:else}
				Deploy
			{/if}
		</Button>
	{/if}
</div>
