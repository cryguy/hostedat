import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: 'dist',
			assets: 'dist',
			fallback: 'index.html',
			precompress: false
		}),
		alias: {
			$components: 'src/lib/components',
			$stores: 'src/lib/stores',
			$api: 'src/lib/api'
		}
	},
	preprocess: vitePreprocess()
};

export default config;
