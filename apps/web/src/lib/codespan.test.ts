import { Marked } from 'marked';
import { describe, expect, it } from 'vitest';
import { codespan } from './codespan';
import privacy from '../content/legal/privacy.md?raw';

describe('codespan', () => {
	const marked = new Marked({ renderer: { codespan } });

	it('keeps a command in the legal pages on one line', () => {
		// LegalPage used marked's default, and /privacy at 1440 split
		// `repose-notify` as "repose-" / "notify".
		const html = marked.parse(privacy) as string;
		expect(html).toContain('<code class="nobreak">repose-notify</code>');
	});

	it('leaves a long span to wrap and escapes what it keeps', () => {
		const long = 'x'.repeat(31);
		expect(marked.parseInline(`\`${long}\``)).toBe(`<code>${long}</code>`);
		expect(marked.parseInline('`a<b`')).toBe('<code class="nobreak">a&lt;b</code>');
	});
});
