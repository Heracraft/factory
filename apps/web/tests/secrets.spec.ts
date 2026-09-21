// Checklist: "Secrets: add via text and via file, list, delete; values
// never appear in the DOM after save."
import { test, expect } from '@playwright/test';
import { signIn, createProject, apiURLFromEnv } from './helpers';

test.beforeEach(async ({ page }) => {
	await signIn(page);
});

test('add via text, list, and delete; the value never reaches the DOM', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-app',
		remote_url: 'github.com/heracraft/secrets-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('DATABASE_URL');
	await page.getByPlaceholder('Value').fill('postgres://user:hunter2@db/app');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	const secretRow = page.getByText('DATABASE_URL', { exact: true });
	await expect(secretRow).toBeVisible({ timeout: 5_000 });
	expect(await page.content()).not.toContain('hunter2');

	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Delete' }).click();
	await expect(secretRow).toHaveCount(0, { timeout: 5_000 });
});

test('a name that does not match the allowed pattern is rejected client-side', async ({
	page
}) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-invalid-app',
		remote_url: 'github.com/heracraft/secrets-invalid-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('lowercase_name');
	await page.getByPlaceholder('Value').fill('x');
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText(/\[A-Z\]\[A-Z0-9_\]/)).toBeVisible();
});

test('a value over 64 KB is rejected client-side before it reaches the api', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-huge-app',
		remote_url: 'github.com/heracraft/secrets-huge-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('BIG_VALUE');
	await page.getByPlaceholder('Value').fill('x'.repeat(70_000));
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText('Secret values are limited to 64 KB.')).toBeVisible();
	await expect(page.getByText('BIG_VALUE')).toHaveCount(0);
});

// The other half of the checklist row: "add via text **and via file**".
// The file path is not the text path with a different source — it reads
// an ArrayBuffer and base64s the bytes, so a value that is not valid
// UTF-8 (a key file, the common case) survives only on this path.
test('add via file stores the bytes, and they never reach the DOM', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-file-app',
		remote_url: 'github.com/heracraft/secrets-file-app'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	// A byte sequence that is deliberately not valid UTF-8, so a path that
	// round-tripped through a string would corrupt it.
	const bytes = Buffer.from([0x00, 0xff, 0xfe, 0x41, 0x42, 0x43, 0x80, 0x0a]);
	await page.getByPlaceholder('NAME').fill('SERVICE_ACCOUNT_KEY');
	await page.setInputFiles('input[type=file]', {
		name: 'key.bin',
		mimeType: 'application/octet-stream',
		buffer: bytes
	});
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText('SERVICE_ACCOUNT_KEY', { exact: true })).toBeVisible({
		timeout: 5_000
	});

	// What the api received is the base64 of those exact bytes, and none of
	// it is on the page afterwards.
	const b64 = bytes.toString('base64');
	expect(await page.content()).not.toContain(b64);
	expect(await page.content()).not.toContain('ABC');

	const res = await page.request.get(`${apiURLFromEnv()}/projects/${p.id}/secrets`, {
		headers: { Authorization: 'Bearer playwright' }
	});
	const names = (await res.json()).map((s: { name: string }) => s.name);
	expect(names).toContain('SERVICE_ACCOUNT_KEY');
});

// A file over the cap is refused on the file path too, not only the
// textarea's (08 §6, "Secret value over 64 KB").
test('a file over 64 KB is rejected client-side', async ({ page }) => {
	const p = await createProject(apiURLFromEnv(), {
		name: 'secrets-big-file',
		remote_url: 'github.com/heracraft/secrets-big-file'
	});
	await page.goto(`/projects/${p.id}/secrets`);

	await page.getByPlaceholder('NAME').fill('BIG_FILE');
	await page.setInputFiles('input[type=file]', {
		name: 'big.bin',
		mimeType: 'application/octet-stream',
		buffer: Buffer.alloc(64 * 1024 + 1, 7)
	});
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText('Secret values are limited to 64 KB.')).toBeVisible();
	await expect(page.getByText('BIG_FILE', { exact: true })).toHaveCount(0);
});
