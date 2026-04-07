import { expect, test } from 'playwright/test';

import { createTripGenerator, fetchBBox } from '../performance/loadbot/trip-generator.mjs';

const SERVER_URL = process.env.END2END_SERVER_URL || 'http://127.0.0.1:8080';
const candidateAttempts = 30;

async function findDrivableTrip(request: import('playwright/test').APIRequestContext) {
  const bbox = await fetchBBox(SERVER_URL);
  const nextTrip = createTripGenerator(bbox, 0.18, 0.08);

  for (let attempt = 0; attempt < candidateAttempts; attempt += 1) {
    const trip = nextTrip();
    const response = await request.post(`${SERVER_URL}/route`, {
      data: {
        src_lat: trip.source.lat,
        src_lon: trip.source.lng,
        dst_lat: trip.dest.lat,
        dst_lon: trip.dest.lng,
        alternatives: 2,
      },
    });

    if (!response.ok()) {
      continue;
    }

    const payload = await response.json();
    if (Array.isArray(payload?.routes) && payload.routes.length > 0) {
      return trip;
    }
  }

  throw new Error(`Could not find a drivable trip after ${candidateAttempts} attempts`);
}

async function openLoadbotPage(page: import('playwright/test').Page) {
  await page.goto('/navigation.html?loadbot=1');
  await page.waitForFunction(() => Boolean((window as Window & { __loadbot?: unknown }).__loadbot));
}

test('route planning and drive controls work end-to-end', async ({ page, request }) => {
  const trip = await findDrivableTrip(request);

  await openLoadbotPage(page);
  await page.evaluate(async (currentTrip) => {
    return (window as Window & { __loadbot: { planTrip: (trip: unknown) => Promise<unknown> } }).__loadbot.planTrip({
      source: currentTrip.source,
      dest: currentTrip.dest,
    });
  }, trip);

  await expect(page.locator('#route-panel')).toBeVisible();
  await expect(page.locator('#eta-value')).not.toHaveText('--');
  await expect(page.locator('#distance-value')).not.toHaveText('--');

  await page.click('#start-drive-btn');
  await expect(page.locator('#driving-hud')).toBeVisible();

  await page.waitForFunction(() => {
    const bridge = (window as Window & { __loadbot?: { state: () => { sessionId: string | null } } }).__loadbot;
    return Boolean(bridge?.state().sessionId);
  });

  await page.click('#stop-drive-btn');
  await expect(page.locator('#driving-hud')).toBeHidden();
  await expect(page.locator('#route-panel')).toBeVisible();
});
