import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// adapter-node, not adapter-auto: the Dockerfile (5.6) runs `node
		// build` directly, which adapter-auto cannot target ahead of time.
		adapter: adapter()
	}
};

export default config;
