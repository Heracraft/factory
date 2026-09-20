import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// This dev box is reached over Tailscale, not localhost.
		host: '0.0.0.0'
	},
	preview: {
		host: '0.0.0.0'
	},
	test: {
		// Playwright owns everything under tests/ (its own *.spec.ts files);
		// Vitest only sees the co-located unit tests under src/.
		include: ['src/**/*.test.ts'],
		environment: 'jsdom'
	}
});
