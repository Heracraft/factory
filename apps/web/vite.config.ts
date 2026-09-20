import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// This dev box is reached over Tailscale, not localhost.
		host: '0.0.0.0'
	},
	preview: {
		host: '0.0.0.0'
	}
});
