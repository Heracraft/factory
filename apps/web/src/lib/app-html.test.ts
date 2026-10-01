import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

// app.html paints the page before routes/layout.css arrives (DECISIONS
// I-331), with the page and ink colours written out by hand. If the two
// files drift, every page flashes the old colour on load, in one scheme or
// both. This reads the colours out of both files and holds them equal.
const read = (rel: string) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');
const appHtml = read('../app.html');
const css = read('../routes/layout.css');

/** The value of a custom property in a block of CSS, with var() resolved. */
function resolve(block: string, name: string): string {
	const m = new RegExp(`${name}:\\s*([^;]+);`).exec(block);
	if (!m) throw new Error(`${name} not found`);
	const value = m[1].trim();
	const ref = /^var\((--[\w-]+)\)$/.exec(value);
	return ref ? resolve(css, ref[1]) : value.toLowerCase();
}

function schemeBlocks(): { light: string; dark: string } {
	const base = css.slice(css.indexOf('@layer base'));
	const light = base.slice(
		base.indexOf(':root {'),
		base.indexOf('@media (prefers-color-scheme: dark)')
	);
	const darkStart = base.indexOf('@media (prefers-color-scheme: dark)');
	const dark = base.slice(darkStart, base.indexOf('body {', darkStart));
	return { light, dark };
}

describe('app.html', () => {
	const { light, dark } = schemeBlocks();
	const [lightHtml, darkHtml] = appHtml.split('@media (prefers-color-scheme: dark)');

	it('paints the page and the text in layout.css colours before it loads', () => {
		const bg = (s: string) => /background:\s*(#[0-9a-f]{6})/i.exec(s)?.[1].toLowerCase();
		const fg = (s: string) => /\bcolor:\s*(#[0-9a-f]{6})/i.exec(s)?.[1].toLowerCase();
		expect(bg(lightHtml)).toBe(resolve(light, '--page'));
		expect(fg(lightHtml)).toBe(resolve(light, '--ink'));
		expect(bg(darkHtml)).toBe(resolve(dark, '--page'));
		expect(fg(darkHtml)).toBe(resolve(dark, '--ink'));
	});

	it('gives the browser chrome the page colour in each scheme', () => {
		const theme = (scheme: string) =>
			new RegExp(
				`theme-color" content="(#[0-9a-f]{6})" media="\\(prefers-color-scheme: ${scheme}\\)`,
				'i'
			)
				.exec(appHtml)?.[1]
				.toLowerCase();
		expect(theme('light')).toBe(resolve(light, '--page'));
		expect(theme('dark')).toBe(resolve(dark, '--page'));
	});
});
