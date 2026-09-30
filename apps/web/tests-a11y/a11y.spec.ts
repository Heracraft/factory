// Lighthouse accessibility audits of every page a visitor or a signed-in
// user can reach, in both colour schemes and at two widths.
//
// 08 §9 names /projects and /projects/[id]/config and a score of 90. A score
// is an average, though, and one failed color-contrast audit still scores
// above 90, which is how muted text under 4.5:1 shipped past this gate. So a
// run fails on the aggregate under 90 and also on any binary audit that
// scores 0, unless KNOWN_FAILURES names that audit on that page with a
// reason.
//
// The signed-in pages need a session, and Lighthouse cannot sign in, so this
// runs Lighthouse against the Chromium instance Playwright has already
// signed in: the browser is launched with a CDP port, the test signs in, and
// the Lighthouse CLI attaches to that same browser with --port.
// tests-a11y/fixtures.ts explains why that browser is a persistent context.
// Each report's final URL is checked against the audited one, so an audit
// that was bounced to the landing page fails instead of scoring the wrong
// page.
//
// It audits the same bundle production serves (`pnpm build` output, run by
// tests/fixtures.ts), against internal/fakes/api rather than the deployed
// api. Accessibility is a property of the markup, which the api does not
// change; what a run against production would add is the real content's
// text lengths and colours, which the fake's fixtures already mirror.
import { execFile } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';
import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';
import {
	signIn,
	createProject,
	apiURLFromEnv,
	setBilling,
	resetBilling,
	BASE_URL
} from '../tests/helpers';
import { CDP_PORT } from './cdp';

const run = promisify(execFile);

const MINIMUM = 90;
const OUT_DIR = path.resolve(import.meta.dirname, '../test-results/lighthouse');

/**
 * Binary audits allowed to fail, by Lighthouse audit id, each with the pages
 * it may fail on (paths as in PAGES, or '*') and why it is tolerated. An
 * entry is a debt with a reason, never a way to quiet a run: delete it when
 * the fix lands. Empty means every binary audit must pass everywhere.
 */
const KNOWN_FAILURES: Record<string, { pages: string[]; reason: string }> = {};

/** The two widths docs/LANDING.md judges a page at. */
const WIDTHS = {
	desktop: [
		'--form-factor=desktop',
		'--screenEmulation.mobile=false',
		'--screenEmulation.width=1440',
		'--screenEmulation.height=900',
		'--screenEmulation.deviceScaleFactor=1'
	],
	mobile: [
		'--form-factor=mobile',
		'--screenEmulation.mobile',
		'--screenEmulation.width=390',
		'--screenEmulation.height=844',
		'--screenEmulation.deviceScaleFactor=3'
	]
} as const;

interface Target {
	/** The path as reported, with the project id left as [id]. */
	name: string;
	signedIn: boolean;
	url: () => string;
	/** Waits in the test's own tab until the page has rendered its content. */
	ready: (page: Page) => Promise<void>;
	/** Puts the fake api into the state the audit should see. */
	setup?: () => Promise<void>;
	teardown?: () => Promise<void>;
}

let projectId = '';

const heading = (page: Page) => expect(page.locator('main h1').first()).toBeVisible();

// Public pages come first: each worker starts signed out (a fresh profile),
// and these are audited as a visitor sees them. The landing page renders
// differently once signed in.
const PAGES: Target[] = [
	{
		name: '/',
		signedIn: false,
		url: () => '/',
		ready: (page) => expect(page.getByRole('button', { name: 'Get started' })).toBeVisible()
	},
	{ name: '/docs', signedIn: false, url: () => '/docs', ready: heading },
	// A docs page with prompts, output and a copy button in its code blocks.
	{ name: '/docs/lifecycle', signedIn: false, url: () => '/docs/lifecycle', ready: heading },
	{ name: '/privacy', signedIn: false, url: () => '/privacy', ready: heading },
	{ name: '/terms', signedIn: false, url: () => '/terms', ready: heading },
	{ name: '/refunds', signedIn: false, url: () => '/refunds', ready: heading },
	{
		name: '/projects',
		signedIn: true,
		url: () => '/projects',
		ready: (page) => expect(page.locator('table')).toBeVisible()
	},
	{
		name: '/projects/[id]',
		signedIn: true,
		url: () => `/projects/${projectId}`,
		ready: (page) =>
			expect(page.getByRole('heading', { level: 1, name: /^a11y-app-/ })).toBeVisible()
	},
	{
		name: '/projects/[id]/config',
		signedIn: true,
		url: () => `/projects/${projectId}/config`,
		ready: (page) => expect(page.getByRole('button', { name: 'Menu' })).toBeVisible()
	},
	{
		name: '/projects/[id]/secrets',
		signedIn: true,
		url: () => `/projects/${projectId}/secrets`,
		ready: (page) => expect(page.getByRole('heading', { level: 1, name: 'Secrets' })).toBeVisible()
	},
	{
		name: '/settings',
		signedIn: true,
		url: () => '/settings',
		ready: (page) => expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
	},
	{
		name: '/account',
		signedIn: true,
		url: () => '/account',
		ready: (page) => expect(page.getByRole('heading', { level: 1, name: 'Account' })).toBeVisible()
	},
	// With no plan the page shows the three plan cards, its densest state;
	// the default fixture state (billing off) is a single sentence.
	{
		name: '/billing',
		signedIn: true,
		url: () => '/billing',
		ready: (page) => expect(page.getByRole('heading', { level: 2, name: 'Solo' })).toBeVisible(),
		setup: () => setBilling({ mode: 'none' }),
		teardown: resetBilling
	}
];

interface Audit {
	id: string;
	title: string;
	score: number | null;
	scoreDisplayMode: string;
	details?: { items?: { node?: { selector?: string; snippet?: string } }[] };
}

interface Result {
	score: number;
	/** Binary audits that scored 0, as "id: title (first offending nodes)". */
	failed: { id: string; line: string }[];
	finalUrl: string;
}

/**
 * Runs the Lighthouse CLI against the already-open browser and returns the
 * accessibility score out of 100 and the failed binary audits, leaving the
 * HTML and JSON reports on disk as the evidence 08 §9 asks for.
 */
async function audit(file: string, url: string, width: keyof typeof WIDTHS): Promise<Result> {
	const out = path.join(OUT_DIR, file);
	await run(
		'lighthouse',
		[
			url,
			`--port=${CDP_PORT}`,
			'--only-categories=accessibility',
			'--output=json',
			'--output=html',
			`--output-path=${out}`,
			'--quiet',
			'--disable-full-page-screenshot',
			// The session lives in this origin's localStorage; Lighthouse's
			// default storage reset would sign the browser out between runs.
			'--disable-storage-reset',
			// Accessibility audits read the DOM, not timings, so simulated
			// throttling only makes the run slower.
			'--throttling-method=provided',
			...WIDTHS[width]
		],
		{ timeout: 180_000, maxBuffer: 32 * 1024 * 1024 }
	);
	const report = JSON.parse(fs.readFileSync(`${out}.report.json`, 'utf8'));
	const audits = Object.values(report.audits as Record<string, Audit>);
	const failed = audits
		.filter((a) => a.scoreDisplayMode === 'binary' && a.score === 0)
		.map((a) => {
			const nodes = (a.details?.items ?? [])
				.slice(0, 3)
				.map((i) => i.node?.selector ?? i.node?.snippet)
				.filter(Boolean);
			return {
				id: a.id,
				line: `${a.id}: ${a.title}${nodes.length ? ` (${nodes.join('; ')})` : ''}`
			};
		});
	return {
		score: Math.round(report.categories.accessibility.score * 100),
		failed,
		finalUrl: report.finalDisplayedUrl
	};
}

const signedInContexts = new WeakSet<object>();

async function ensureSignedIn(page: Page): Promise<void> {
	if (signedInContexts.has(page.context())) return;
	await signIn(page);
	signedInContexts.add(page.context());
}

test.beforeAll(async () => {
	fs.mkdirSync(OUT_DIR, { recursive: true });
	const p = await createProject(apiURLFromEnv(), {
		name: 'a11y-app',
		remote_url: 'github.com/heracraft/a11y-app'
	});
	projectId = p.id;
});

for (const target of PAGES) {
	for (const width of Object.keys(WIDTHS) as (keyof typeof WIDTHS)[]) {
		test(`${target.name} at ${width} width passes every accessibility audit`, async ({
			page,
			scheme
		}) => {
			if (target.signedIn) await ensureSignedIn(page);
			await target.setup?.();
			try {
				const pathname = target.url();
				await page.goto(pathname);
				await target.ready(page);

				const slug = target.name.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '') || 'landing';
				const result = await audit(`${slug}-${scheme}-${width}`, `${BASE_URL}${pathname}`, width);
				console.log(`${target.name} ${scheme} ${width}: ${result.score}`);

				// A bounce to the landing page (signed out) would otherwise be
				// scored as if it were this page.
				expect(new URL(result.finalUrl).pathname, 'the page Lighthouse audited').toBe(pathname);
				expect(result.score).toBeGreaterThanOrEqual(MINIMUM);

				const unexpected = result.failed.filter((f) => {
					const known = KNOWN_FAILURES[f.id];
					return !known || !(known.pages.includes('*') || known.pages.includes(target.name));
				});
				expect(
					unexpected.map((f) => f.line),
					'failed binary audits'
				).toEqual([]);
			} finally {
				await target.teardown?.();
			}
		});
	}
}
