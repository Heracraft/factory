import { config } from '@repo/eslint-config/index.js';

export default [
	// The production build output — generated code, never authored here.
	{ ignores: ['build/**'] },
	...config
];
