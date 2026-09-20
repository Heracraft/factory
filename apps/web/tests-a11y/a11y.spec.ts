// Lighthouse accessibility scores for the two pages 08 §9 names:
// /projects and /projects/[id]/config.
//
// Both are behind a sign-in, and Lighthouse cannot sign in, so this runs
// Lighthouse against the Chromium instance Playwright has already signed
// in: the browser is launched with a CDP port, the test signs in, and the
// Lighthouse CLI attaches to that same browser with --port. The audited
// pages therefore have a session, a real project and real data.
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
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv, BASE_URL } from '../tests/helpers';
import { CDP_PORT } from './cdp';

const run = promisify(execFile);

const MINIMUM = 90;
const OUT_DIR = path.resolve(import.meta.dirname, '../test-results/lighthouse');

let projectId: string;

test.beforeAll(async () => {
	fs.mkdirSync(OUT_DIR, { recursive: true });
	const p = await createProject(apiURLFromEnv(), {
		name: 'a11y-app',
		remote_url: 'github.com/heracraft/a11y-app'
	});
	projectId = p.id;
});

/**
 * Runs the Lighthouse CLI against the already-open browser and returns the
 * accessibility score out of 100, leaving the HTML report on disk as the
 * evidence 08 §9 asks for.
 */
async function accessibilityScore(name: string, url: string): Promise<number> {
	const out = path.join(OUT_DIR, name);
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
			'--disable-full-page-screenshot'
		],
		{ timeout: 180_000, maxBuffer: 32 * 1024 * 1024 }
	);
	const report = JSON.parse(fs.readFileSync(`${out}.report.json`, 'utf8'));
	const score = Math.round(report.categories.accessibility.score * 100);
	const failed = Object.values(report.audits as Record<string, Record<string, unknown>>)
		.filter((a) => a.scoreDisplayMode === 'binary' && a.score === 0)
		.map((a) => a.id);
	if (failed.length) console.log(`${name}: failing audits ${failed.join(', ')}`);
	return score;
}

// Not one of §9's two rows, but it is the page every visitor sees first
// and it costs one audit to keep honest.
test('accessibility of the landing page is at least 90', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('button', { name: 'Sign in with GitHub' })).toBeVisible();
	const score = await accessibilityScore('landing', `${BASE_URL}/`);
	console.log(`/ accessibility: ${score}`);
	expect(score).toBeGreaterThanOrEqual(MINIMUM);
});

test('accessibility of /projects is at least 90', async ({ page }) => {
	await signIn(page);
	await page.goto('/projects');
	await expect(page.locator('table')).toBeVisible();
	const score = await accessibilityScore('projects', `${BASE_URL}/projects`);
	console.log(`/projects accessibility: ${score}`);
	expect(score).toBeGreaterThanOrEqual(MINIMUM);
});

test('accessibility of /projects/[id]/config is at least 90', async ({ page }) => {
	await signIn(page);
	await page.goto(`/projects/${projectId}/config`);
	await expect(page.getByRole('button', { name: 'Menu' })).toBeVisible();
	const score = await accessibilityScore('config', `${BASE_URL}/projects/${projectId}/config`);
	console.log(`/projects/[id]/config accessibility: ${score}`);
	expect(score).toBeGreaterThanOrEqual(MINIMUM);
});
