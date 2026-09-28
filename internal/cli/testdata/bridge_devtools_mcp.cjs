// Runs the guest's chrome-devtools-mcp against the bridge's front and
// calls its tools over MCP's stdio, as an agent does. Used by
// TestBridgeWithChromeDevtoolsMCP. Args: the server's entry script, the
// front's port, an allowed URL, a URL off the list.
const { spawn } = require('child_process');
const [entry, port, okURL, otherURL] = process.argv.slice(2);
const child = spawn(process.execPath, [entry, '--browserUrl', `http://127.0.0.1:${port}`, '--no-usage-statistics'], { stdio: ['pipe', 'pipe', 'inherit'] });
let buf = '';
const waiting = new Map();
child.stdout.on('data', (d) => {
  buf += d;
  let i;
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i);
    buf = buf.slice(i + 1);
    if (!line.trim()) continue;
    const m = JSON.parse(line);
    if (m.id && waiting.has(m.id)) { waiting.get(m.id)(m); waiting.delete(m.id); }
  }
});
let next = 1;
const rpc = (method, params) => new Promise((resolve) => {
  const id = next++;
  waiting.set(id, resolve);
  child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
});
const text = (r) => (r.result && r.result.content || []).map((c) => c.text || '').join('\n') + (r.error ? 'ERROR ' + r.error.message : '');
(async () => {
  await rpc('initialize', { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'bridge-test', version: '1' } });
  child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
  const call = async (name, args) => text(await rpc('tools/call', { name, arguments: args }));
  const out = {};
  out.newPage = await call('new_page', { url: okURL });
  out.other = await call('navigate_page', { pageId: 1, type: 'url', url: otherURL });
  out.list = await call('list_pages', {});
  console.log(JSON.stringify(out));
  child.kill();
  process.exit(0);
})().catch((e) => { console.error('FAIL', e); child.kill(); process.exit(1); });
