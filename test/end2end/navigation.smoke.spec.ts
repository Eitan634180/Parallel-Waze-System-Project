import { expect, test } from 'playwright/test';

import { findDrivableTrip, openLoadbotPage, planTripThroughUi } from './test-helpers';

type LoadbotPlanBridge = {
  state?: () => { currentRouteId: string | null; currentRouteIndex: number; sessionId: string | null };
};

test('route planning and drive controls work end-to-end', async ({ page, request }) => {
  const trip = await findDrivableTrip(request);

  await openLoadbotPage(page);
  await planTripThroughUi(page, trip);

  await expect(page.locator('#route-panel')).toBeVisible();
  await expect(page.locator('#eta-value')).not.toHaveText('--');
  await expect(page.locator('#distance-value')).not.toHaveText('--');
  await expect(page.locator('.route-option-card').first()).toBeVisible();

  await page.click('#start-drive-btn');
  await expect(page.locator('#driving-hud')).toBeVisible();

  await page.waitForFunction(() => {
    const bridge = (window as unknown as { __loadbot?: LoadbotPlanBridge }).__loadbot;
    return Boolean(bridge?.state?.().sessionId);
  });

  await page.click('#stop-drive-btn');
  await expect(page.locator('#driving-hud')).toBeHidden();
  await expect(page.locator('#route-panel')).toBeVisible();
});

test('alternative route selection updates the active route before driving', async ({ page, request }) => {
  const trip = await findDrivableTrip(request, 2);

  await openLoadbotPage(page);
  await planTripThroughUi(page, trip);

  const routeCards = page.locator('.route-option-card');
  await expect(routeCards.nth(1)).toBeVisible();
  const initialState = await page.evaluate(() => {
    const bridge = (window as unknown as { __loadbot?: LoadbotPlanBridge }).__loadbot;
    return bridge?.state?.();
  });
  await page.click('#route-option-1');

  await page.waitForFunction((initialRouteId) => {
    const bridge = (window as unknown as { __loadbot?: LoadbotPlanBridge }).__loadbot;
    const snapshot = bridge?.state?.();
    return snapshot?.currentRouteIndex === 1 && snapshot.currentRouteId !== initialRouteId;
  }, initialState?.currentRouteId ?? null);

  await expect(page.locator('#route-option-1')).toHaveClass(/active/);
  await expect(page.locator('#eta-value')).not.toHaveText('--');
});
