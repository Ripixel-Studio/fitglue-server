import express, { Request, Response } from 'express';
import { chromium, Browser } from 'playwright';

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 8080;

let browser: Browser | null = null;

async function getBrowser(): Promise<Browser> {
  if (!browser) {
    browser = await chromium.launch({
      headless: true,
      args: [
        '--disable-dev-shm-usage',
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-blink-features=AutomationControlled',
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
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36',
      locale: 'en-GB',
      timezoneId: 'Europe/London',
      // Pre-set cookie consent so parkrun doesn't show the banner
      storageState: {
        cookies: [
          {
            name: 'cookie-consent-agreed',
            value: '1',
            domain: '.parkrun.org.uk',
            path: '/',
            expires: Math.floor(Date.now() / 1000) + 365 * 24 * 3600,
            httpOnly: false,
            secure: false,
            sameSite: 'Lax',
          },
          {
            name: 'OptanonAlertBoxClosed',
            value: new Date().toISOString(),
            domain: '.parkrun.org.uk',
            path: '/',
            expires: Math.floor(Date.now() / 1000) + 365 * 24 * 3600,
            httpOnly: false,
            secure: false,
            sameSite: 'Lax',
          },
          {
            name: 'OptanonConsent',
            value: 'isIABGlobal=false&datestamp=' + encodeURIComponent(new Date().toUTCString()) + '&version=6.33.0&landingPath=NotLandingPage&groups=C0001%3A1%2CC0002%3A1%2CC0003%3A1%2CC0004%3A1&hosts=&AwaitingReconsent=false',
            domain: '.parkrun.org.uk',
            path: '/',
            expires: Math.floor(Date.now() / 1000) + 365 * 24 * 3600,
            httpOnly: false,
            secure: false,
            sameSite: 'Lax',
          },
        ],
        origins: [],
      },
    });

    // Hide webdriver fingerprint (runs in browser context, not Node)
    await context.addInitScript(`
      Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
      delete window.__playwright;
      delete window.__pw_manual;
    `);

    const page = await context.newPage();

    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 });

    try {
      await page.waitForLoadState('networkidle', { timeout: 10000 });
    } catch {
      console.log('[parkrun-fetcher] Network idle timeout, continuing anyway');
    }

    // Dismiss cookie consent banner if still present (OneTrust / CivicUK / generic)
    const consentSelectors = [
      'button#onetrust-accept-btn-handler',
      'button.onetrust-accept-btn-handler',
      'button[id*="accept-all"]',
      'button[class*="accept-all"]',
      'button:has-text("Accept All Cookies")',
      'button:has-text("Accept all cookies")',
      'button:has-text("Accept All")',
      'button:has-text("Accept Cookies")',
      'button:has-text("I Accept")',
      '[data-testid="cookie-accept"]',
    ];
    for (const sel of consentSelectors) {
      try {
        await page.click(sel, { timeout: 1500 });
        console.log(`[parkrun-fetcher] Dismissed consent banner via: ${sel}`);
        await page.waitForTimeout(1000);
        break;
      } catch {
        // not found, try next
      }
    }

    // Wait for results table
    try {
      await page.waitForSelector('table tbody tr', { timeout: 8000 });
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

    if (html.length < 5000) {
      console.warn(`[parkrun-fetcher] Response suspiciously small (${html.length} bytes) — likely a consent/bot wall`);
    }

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
