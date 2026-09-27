import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// adapter-node, not adapter-auto: the Dockerfile (5.6) runs `node
		// build` directly, which adapter-auto cannot target ahead of time.
		adapter: adapter(),
		// The one Content-Security-Policy the app has, set here because no
		// layer in front of it (Coolify's Traefik, the edge) sets one. Its
		// point is script-src and frame-src: the only third-party script is
		// Paddle.js on the billing page and the only frame is Paddle's
		// checkout overlay (DECISIONS I-289). SvelteKit nonces its own
		// inline start script (mode auto; hashes on the prerendered legal
		// pages). connect-src cannot name the api and Logto, which are
		// runtime PUBLIC_* values, so it allows https and the loopback the
		// Playwright fixtures listen on. Svelte writes style attributes, so
		// style-src keeps 'unsafe-inline'.
		csp: {
			mode: 'auto',
			directives: {
				'default-src': ['self'],
				'script-src': ['self', 'https://cdn.paddle.com', 'https://*.paddle.com'],
				'style-src': ['self', 'unsafe-inline', 'https://fonts.googleapis.com'],
				'font-src': ['self', 'https://fonts.gstatic.com'],
				'img-src': ['self', 'data:', 'https:'],
				'connect-src': [
					'self',
					'https:',
					'wss:',
					'http://127.0.0.1:*',
					'http://localhost:*',
					'ws://127.0.0.1:*',
					'ws://localhost:*'
				],
				'frame-src': [
					'https://*.paddle.com',
					'https://buy.paddle.com',
					'https://sandbox-buy.paddle.com'
				],
				'object-src': ['none'],
				'base-uri': ['self']
			}
		}
	}
};

export default config;
