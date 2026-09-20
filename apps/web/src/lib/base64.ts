/** Base64-encodes text or binary data, UTF-8 safe (`btoa` alone is not). */
export function encodeBase64(input: string | ArrayBuffer): string {
	const bytes = typeof input === 'string' ? new TextEncoder().encode(input) : new Uint8Array(input);
	let binary = '';
	for (const b of bytes) binary += String.fromCharCode(b);
	return btoa(binary);
}
