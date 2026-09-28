// Drives the bridge's front the way Playwright MCP does
// (connectOverCDP to http://127.0.0.1:PORT, which reads /json/version).
// Used by TestBridgeWithPlaywright. Args: playwright-core dir, front
// port, an allowed URL, a URL off the list.
const [pwDir, port, okURL, otherURL] = process.argv.slice(2);
const { chromium } = require(pwDir);
const first = (e) => String(e.message).split('\n')[0];
(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  const ctx = browser.contexts()[0];
  const pages = ctx.pages().map((p) => p.url());
  const page = await ctx.newPage();
  await page.goto(okURL);
  const title = await page.title();
  let otherErr = '';
  try { await page.goto(otherURL); } catch (e) { otherErr = first(e); }
  await page.goto(okURL);
  await page.evaluate((u) => { document.body.innerHTML = `<a id="a" href="${u}">x</a>`; }, otherURL + 'click');
  let clickErr = '';
  try {
    await Promise.all([page.waitForURL(/click/, { timeout: 3000 }), page.click('#a')]);
  } catch (e) { clickErr = first(e); }
  const shot = (await page.screenshot()).length;
  let cookieErr = '';
  try { await ctx.cookies(); } catch (e) { cookieErr = first(e); }
  await page.close();
  await browser.close();
  console.log(JSON.stringify({ pages, title, otherErr, clickErr, shot, cookieErr }));
})().catch((e) => { console.error('FAIL', e); process.exit(1); });
