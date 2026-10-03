import { describe, expect, it } from 'vitest';
import { DOCS } from './docs';
import { absoluteLinks, docMarkdown, llmsTxt, ORIGIN } from './llms';

describe('llms.txt (I-412)', () => {
	it('lists every page as a fetchable .md', () => {
		const txt = llmsTxt();
		expect(txt.startsWith('# repose\n')).toBe(true);
		for (const d of DOCS) expect(txt).toContain(`(${ORIGIN}/docs/${d.slug}.md): `);
		expect(txt).toContain('~/.repose/cli-version');
	});

	it('makes docs links absolute .md links, anchors kept', () => {
		expect(absoluteLinks('see [Sync](/docs/sync#where-the-checkout-is) and [docs](/docs).')).toBe(
			`see [Sync](${ORIGIN}/docs/sync.md#where-the-checkout-is) and [docs](${ORIGIN}/docs/index.md).`
		);
		expect(absoluteLinks('[terms](/terms) [x](https://example.com/docs/a)')).toBe(
			'[terms](/terms) [x](https://example.com/docs/a)'
		);
	});

	it('a page is its title, description and body without frontmatter', () => {
		const md = docMarkdown(DOCS.find((d) => d.slug === 'cli')!);
		expect(md.startsWith('# CLI reference\n\n> ')).toBe(true);
		expect(md).not.toContain('\nsection: ');
		expect(md).toContain('repose exec');
		expect(md).not.toMatch(/\]\(\/docs/);
	});
});
