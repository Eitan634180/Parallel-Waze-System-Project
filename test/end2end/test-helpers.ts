import type { APIRequestContext, Page } from 'playwright/test';

import { testConfig } from '../config/test-config.mjs';
import { createTripGenerator, fetchBBox } from '../performance/loadbot/trip-generator.mjs';

export const e2eServerURL = testConfig.server.url;

type TripPoint = {
  lat: number;
  lng: number;
};

type PlannedTrip = {
  source: TripPoint;
  dest: TripPoint;
  routes: Array<{ id: string }>;
};

function coordinateQuery(point: TripPoint) {
  return `${point.lat.toFixed(5)}, ${point.lng.toFixed(5)}`;
}

async function chooseCoordinateSuggestion(page: Page, inputSelector: string, suggestionsSelector: string, point: TripPoint) {
  await page.fill(inputSelector, coordinateQuery(point));
  await page.locator(`${suggestionsSelector} .suggestion-item`).first().click();
}

export async function findDrivableTrip(request: APIRequestContext, minimumRoutes = 1): Promise<PlannedTrip> {
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
        alternatives: Math.max(0, minimumRoutes - 1),
      },
    });

    if (!response.ok()) {
      continue;
    }

    const payload = await response.json();
    if (Array.isArray(payload?.routes) && payload.routes.length >= minimumRoutes) {
      return {
        ...trip,
        routes: payload.routes,
      };
    }
  }

  throw new Error(`Could not find a drivable trip with ${minimumRoutes} routes after ${testConfig.e2e.candidateAttempts} attempts`);
}

export async function openLoadbotPage(page: Page) {
  await page.goto(testConfig.e2e.loadbotPath);
  await page.waitForFunction(() => Boolean((window as Window & { __loadbot?: unknown }).__loadbot), null, {
    timeout: testConfig.e2e.expectTimeoutMs,
  });
}

export async function planTripThroughUi(page: Page, trip: PlannedTrip) {
  await chooseCoordinateSuggestion(page, '#source-input', '#source-suggestions', trip.source);
  await chooseCoordinateSuggestion(page, '#dest-input', '#dest-suggestions', trip.dest);
  await page.click('#navigate-btn');
  await page.waitForSelector('#route-panel:not(.hidden)');
}
