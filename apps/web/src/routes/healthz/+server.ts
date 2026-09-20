// The only server route in this app (08-dashboard.md checklist): Coolify's
// HEALTHCHECK and rolling-deploy probe. No auth, no api call, no secrets —
// it only proves the Node process is alive and serving.
export function GET() {
	return new Response('ok');
}
