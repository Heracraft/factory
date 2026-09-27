// Paddle.js, loaded on the billing page alone (DECISIONS I-289): the api
// opens a transaction server-side and the browser shows Paddle's overlay
// for it. The subscription itself arrives by webhook, so the page's job
// after `checkout.completed` is to poll GET /billing until it is there.
//
// Against internal/fakes/api (`environment: "fake"`) nothing is loaded:
// a test installs `window.__reposePaddleStub`, which stands for the
// overlay and calls the fake's admin endpoint that plays the webhook's
// part. That keeps the whole flow testable end to end without Paddle.

export const PADDLE_SCRIPT = 'https://cdn.paddle.com/paddle/v2/paddle.js';

interface PaddleEvent {
	name: string;
}

interface PaddleGlobal {
	Environment: { set(env: 'sandbox' | 'production'): void };
	Initialize(opts: { token: string; eventCallback?: (ev: PaddleEvent) => void }): void;
	Checkout: {
		open(opts: {
			transactionId: string;
			settings: {
				displayMode: 'overlay' | 'inline';
				theme: 'light' | 'dark';
				successUrl?: string;
			};
		}): void;
	};
}

/** What a Playwright test installs in place of Paddle's overlay. */
export interface PaddleStub {
	open(opts: { transactionId: string; onCompleted: () => void }): void | Promise<void>;
}

declare global {
	interface Window {
		Paddle?: PaddleGlobal;
		__reposePaddleStub?: PaddleStub;
	}
}

export interface OpenCheckoutOptions {
	transactionId: string;
	clientToken: string;
	environment: 'sandbox' | 'live' | 'fake';
	theme: 'light' | 'dark';
	successUrl: string;
	/** Called on Paddle's `checkout.completed`. */
	onCompleted: () => void;
}

let loading: Promise<PaddleGlobal> | undefined;
let initialized = false;
// Initialize is once per page load; the completion callback of the newest
// checkout is what its event reaches.
let onCompletedRef: (() => void) | undefined;

function loadPaddle(): Promise<PaddleGlobal> {
	if (window.Paddle) return Promise.resolve(window.Paddle);
	loading ??= new Promise<PaddleGlobal>((resolve, reject) => {
		const script = document.createElement('script');
		script.src = PADDLE_SCRIPT;
		script.async = true;
		script.onload = () => {
			if (window.Paddle) resolve(window.Paddle);
			else reject(new Error('Paddle.js loaded without its global'));
		};
		script.onerror = () => {
			loading = undefined;
			reject(new Error('Could not load Paddle.js'));
		};
		document.head.appendChild(script);
	});
	return loading;
}

/** The theme Paddle's overlay should match: the page follows the OS scheme. */
export function pageTheme(): 'light' | 'dark' {
	return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export async function openCheckout(opts: OpenCheckoutOptions): Promise<void> {
	if (opts.environment === 'fake') {
		const stub = window.__reposePaddleStub;
		if (!stub) throw new Error('Paddle is not available here.');
		await stub.open({ transactionId: opts.transactionId, onCompleted: opts.onCompleted });
		return;
	}
	const Paddle = await loadPaddle();
	onCompletedRef = opts.onCompleted;
	if (!initialized) {
		if (opts.environment === 'sandbox') Paddle.Environment.set('sandbox');
		Paddle.Initialize({
			token: opts.clientToken,
			eventCallback: (ev) => {
				if (ev.name === 'checkout.completed') onCompletedRef?.();
			}
		});
		initialized = true;
	}
	Paddle.Checkout.open({
		transactionId: opts.transactionId,
		settings: { displayMode: 'overlay', theme: opts.theme, successUrl: opts.successUrl }
	});
}
