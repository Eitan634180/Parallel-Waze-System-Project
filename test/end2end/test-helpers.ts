import type { APIRequestContext, Page } from 'playwright/test';

import { testConfig } from '../config/test-config.mjs';
import { createTripGenerator, fetchBBox } from '../performance/loadbot/trip-generator.mjs';

export const e2eServerURL = testConfig.server.url;

export async function findDrivableTrip(request: APIRequestContext) {
  const bbox = await fetchBBox(e2eServerURL);
  const nextTrip = createTripGenerator(
    bbox,
    testConfig.e2e.tripInsetFraction,
    testConfig.e2e.tripCommuteDegrees,
    testConfig.tripGeneration,
  );

  for (let attempt = 0; attempt < testConfig.e2e.candidateAttempts; attempt += 1) {
    const trip = nextTrip();
    const response = await request.post(`${e2eServerURL}/route`, {
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

  throw new Error(`Could not find a drivable trip after ${testConfig.e2e.candidateAttempts} attempts`);
}

export async function openLoadbotPage(page: Page) {
  await page.goto(testConfig.e2e.loadbotPath);
  await page.waitForFunction(() => Boolean((window as Window & { __loadbot?: unknown }).__loadbot), null, {
    timeout: testConfig.e2e.expectTimeoutMs,
  });
}
