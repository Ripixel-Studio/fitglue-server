import express, { Request, Response } from 'express';
import { chromium, Browser } from 'playwright';

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 8080;

// Keep browser instance alive for reuse (Cloud Run container lifecycle)
let browser: Browser | null = null;

async function getBrowser(): Promise<Browser> {
  if (!browser) {
    browser = await chromium.launch({
      headless: true,
      args: [
        '--disable-dev-shm-usage', // Required for Docker
        '--no-sandbox',            // Required for Cloud Run
        '--disable-setuid-sandbox',
      ],
    });
  }
  return browser;
}

interface FetchRequest {
  url: string;
}

interface FetchResponse {
  html: string;
  byteLength: number;
  success: boolean;
  error?: string;
}

/**
 * POST /fetch
 * Fetches a URL using a headless browser to bypass JavaScript challenges.
 */
app.post('/fetch', async (req: Request<{}, FetchResponse, FetchRequest>, res: Response<FetchResponse>) => {
  const { url } = req.body;

  if (!url) {
    return res.status(400).json({ html: '', byteLength: 0, success: false, error: 'Missing required field: url' });
  }

  if (!url.startsWith('https://') || !url.includes('parkrun')) {
    return res.status(400).json({ html: '', byteLength: 0, success: false, error: 'Invalid URL: must be a parkrun HTTPS URL' });
  }

  console.log(`[parkrun-fetcher] Fetching: ${url}`);
  const startTime = Date.now();

  try {
    const browserInstance = await getBrowser();
    const context = await browserInstance.newContext({
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    });
    const page = await context.newPage();

    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 });

    try {
      await page.waitForLoadState('networkidle', { timeout: 10000 });
    } catch {
      console.log('[parkrun-fetcher] Network idle timeout, continuing anyway');
    }

    try {
      await page.waitForSelector('table tbody tr', { timeout: 5000 });
      console.log('[parkrun-fetcher] Results table found');
    } catch {
      console.log('[parkrun-fetcher] Results table selector timeout, continuing');
    }

    await page.waitForTimeout(1000);

    let html = '';
    let retries = 3;
    while (retries > 0) {
      try {
        html = await page.content();
        break;
      } catch (contentError) {
        const msg = contentError instanceof Error ? contentError.message : '';
        if (msg.includes('navigating') && retries > 1) {
          console.log(`[parkrun-fetcher] Page still navigating, retrying (${retries - 1} left)`);
          await page.waitForTimeout(1000);
          retries--;
        } else {
          throw contentError;
        }
      }
    }

    await context.close();

    const duration = Date.now() - startTime;
    console.log(`[parkrun-fetcher] Success: ${html.length} bytes in ${duration}ms`);

    return res.json({ html, byteLength: html.length, success: true });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';
    console.error(`[parkrun-fetcher] Error: ${errorMessage}`);
    return res.status(500).json({ html: '', byteLength: 0, success: false, error: errorMessage });
  }
});

app.get('/health', (_req: Request, res: Response) => {
  res.json({ status: 'ok', service: 'parkrun-fetcher' });
});

process.on('SIGTERM', async () => {
  console.log('[parkrun-fetcher] Shutting down...');
  if (browser) await browser.close();
  process.exit(0);
});

app.listen(PORT, () => {
  console.log(`[parkrun-fetcher] Server running on port ${PORT}`);
});
