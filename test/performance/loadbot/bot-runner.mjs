export async function runBot({ browser, clientURL, config, id, nextTrip, shouldStop, onTripEvent }) {
  const context = await browser.newContext({
    viewport: { width: config.viewportWidth, height: config.viewportHeight },
    reducedMotion: 'reduce',
  });

  try {
    const page = await context.newPage();
    await blockNonEssentialAssets(page, config.blockedAssetPatterns);
    page.on('pageerror', (error) => {
      console.error(`bot ${id}: page error`, error);
    });

    await loadBotPage(page, clientURL);

    while (!shouldStop()) {
      try {
        onTripEvent('start');
        await page.evaluate((payload) => window.__loadbot.driveTrip(payload), nextTrip());
        await waitUntilTripEnds(page, shouldStop, config.tripEndPollMs);
        onTripEvent('complete');
      } catch (error) {
        onTripEvent('failure');

        if (String(error.message || '').includes('no route found')) {
          continue;
        }
        console.error(`bot ${id}: trip failed`, error.message);

        try {
          await loadBotPage(page, clientURL);
        } catch (reloadError) {
          console.error(`bot ${id}: reload failed`, reloadError.message);
          return;
        }
      }
    }
  } finally {
    await context.close();
  }
}

async function blockNonEssentialAssets(page, blockedAssetPatterns) {
  await page.route('**/*', (route) => {
    const url = route.request().url();
    const type = route.request().resourceType();
    if (type === 'image' || type === 'media' || type === 'font') {
      return route.abort();
    }
    if (blockedAssetPatterns.some((pattern) => url.includes(pattern))) {
      return route.abort();
    }
    return route.continue();
  });
}

async function loadBotPage(page, clientURL) {
  await page.goto(clientURL, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => Boolean(window.__loadbot));
}

async function waitUntilTripEnds(page, shouldStop, tripEndPollMs) {
  while (!shouldStop()) {
    const state = await page.evaluate(() => window.__loadbot.state());
    if (!state.isDriving) {
      return;
    }
    await sleep(tripEndPollMs);
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
