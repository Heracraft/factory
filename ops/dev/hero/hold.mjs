import { chromium } from '/home/azureuser/projects/factory/node_modules/.pnpm/node_modules/playwright/index.mjs';
const b = await chromium.launch({ executablePath: process.env.CH, args: ['--no-sandbox'] });
const p = await b.newPage(); await p.goto('http://127.0.0.1:5173/join'); 
const q = await b.newPage(); await q.goto('http://127.0.0.1:5173/');
await new Promise(r => setTimeout(r, 260000)); await b.close();
