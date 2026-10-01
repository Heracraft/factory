import { test, expect } from '@playwright/test';

// /docs is public: a signed-out visitor stays on it, can move between pages
// and can search.
test('docs are readable while signed out, with search and prev/next', async ({ page }) => {
	await page.goto('/docs');
	await expect(page).toHaveURL('/docs');
	await expect(page.getByRole('heading', { level: 1, name: 'Quickstart' })).toBeVisible();

	await page.goto('/docs/machine');
	await expect(page).toHaveURL('/docs/machine');
	await expect(page.getByRole('heading', { level: 1, name: 'The machine' })).toBeVisible();
	await page.getByRole('link', { name: /Next\s*Installing software/ }).click();
	await expect(page).toHaveURL('/docs/config');

	await page.getByLabel('Search the docs').fill('ntfy topic');
	const results = page.getByRole('list', { name: 'Search results' });
	await results.getByRole('link').first().click();
	await expect(page).toHaveURL(/\/docs\/notifications/);
});

test('an unknown docs page says so', async ({ page }) => {
	await page.goto('/docs/no-such-page');
	await expect(page.getByRole('heading', { name: 'No such page' })).toBeVisible();
});

// A block with prompts copies its commands only, without the `$ ` or the
// output under them.
test('a code block copies its commands', async ({ page, context }) => {
	await context.grantPermissions(['clipboard-read', 'clipboard-write']);
	await page.goto('/docs/lifecycle');
	const block = page.locator('.doc .code').filter({ hasText: '$ repose stop todo-app' });
	await block.getByRole('button', { name: 'Copy' }).click();
	await expect(block.getByRole('button', { name: 'Copied' })).toBeVisible();
	expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
		'repose stop todo-app\nrepose start todo-app'
	);
});

// The sidebar lists pages only, and from xl up the page's sections are in a
// rail whose column is drawn on every page, so following a link between
// pages moves neither the sidebar, the text nor the rail (I-396).
test('the docs columns hold still between pages, and the rail marks the section', async ({
	page
}) => {
	await page.setViewportSize({ width: 1440, height: 900 });
	const boxes = () =>
		page.evaluate(() => {
			const r = (el: Element | null | undefined) => {
				const b = el!.getBoundingClientRect();
				return [b.x, b.y + scrollY, b.width].map(Math.round).join(',');
			};
			const main = document.querySelector('main');
			return {
				sidebar: r(document.getElementById('docs-nav')),
				main: r(main),
				rail: r(main?.nextElementSibling)
			};
		});

	await page.goto('/docs');
	await expect(page.getByRole('heading', { level: 1, name: 'Quickstart' })).toBeVisible();
	// The text column is max-w-[68ch], and ch is the font's "0": measured
	// before the self-hosted font loads it is the fallback's (605 px on CI
	// instead of 622), so the boxes are read once the fonts are in.
	// document.fonts.ready resolves at once when no load has started yet,
	// which is how the merge of restore-fast failed on CI (2026-10-01), so
	// the boxes are polled until the font has applied.
	await expect
		.poll(boxes)
		.toEqual({ sidebar: '100,57,240', main: '380,57,622', rail: '1116,57,224' });
	const first = await boxes();
	// Layout shifts during the client navigations. The links are clicked
	// from script: a shift within 500ms of real input is left out of the
	// entries' sum (hadRecentInput), and the point is to see every shift.
	await page.evaluate(() => {
		const w = window as unknown as { shifts: number[] };
		w.shifts = [];
		new PerformanceObserver((list) => {
			for (const e of list.getEntries()) w.shifts.push((e as unknown as { value: number }).value);
		}).observe({ type: 'layout-shift' });
	});
	for (const [slug, h1] of [
		['cli', 'CLI reference'],
		['secrets', 'Secrets and security']
	]) {
		await page.evaluate(
			(slug) =>
				document.querySelector<HTMLAnchorElement>(`#docs-nav a[href$="/docs/${slug}"]`)!.click(),
			slug
		);
		await expect(page.getByRole('heading', { level: 1, name: h1 })).toBeVisible();
		expect(await boxes(), slug).toEqual(first);
		// The sidebar holds no section links of the open page.
		await expect(
			page
				.getByRole('complementary', { name: 'Docs menu' })
				.getByRole('list', { name: 'On this page' })
		).toHaveCount(0);
	}

	expect(await page.evaluate(() => (window as unknown as { shifts: number[] }).shifts)).toEqual([]);

	const rail = page.getByRole('navigation', { name: 'On this page' });
	await expect(rail).toBeVisible();
	await rail.getByRole('link', { name: 'What repose stores' }).click();
	await expect(page).toHaveURL('/docs/secrets#what-repose-stores');
	await expect(rail.locator('[aria-current="true"]')).toHaveText('What repose stores');
	await page.evaluate(() => scrollTo(0, 0));
	await expect(rail.locator('[aria-current="true"]')).toHaveText('Store an API key');

	// One jump that carries a heading from below the reading band to above
	// it, never inside (a wheel fling, a scrollbar drag): the observer alone
	// left the rail empty here.
	await page.goto('/docs/cli');
	await page.evaluate(() => {
		const h = document.getElementById('projects')!;
		scrollTo({ top: h.getBoundingClientRect().top + scrollY + 500, behavior: 'instant' });
	});
	await expect(rail.locator('[aria-current="true"]')).toHaveText('Projects');
});

// On a phone the drawer opens scrolled the least that shows the current
// page's link, with the search held at its top: a link near the end of the
// list once put itself a third of the way down and scrolled the search away
// (/docs/cli at 390, I-400).
test("the phone drawer shows the search and the current page's link", async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/docs/cli');
	await page.getByRole('button', { name: 'Open the docs menu' }).click();
	const drawer = page.getByRole('complementary', { name: 'Docs menu' });
	await expect(drawer.getByRole('link', { name: 'CLI reference' })).toBeInViewport({ ratio: 1 });
	const search = drawer.getByLabel('Search the docs');
	await expect(search).toBeInViewport({ ratio: 1 });
	// The search is not under the header, which ends at 57px.
	expect((await search.boundingBox())!.y).toBeGreaterThanOrEqual(57);
});
