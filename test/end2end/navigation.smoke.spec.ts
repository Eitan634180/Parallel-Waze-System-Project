import { expect, test } from 'playwright/test';

import { findDrivableTrip, openLoadbotPage } from './test-helpers';

type LoadbotPlanBridge = {
  planTrip?: (trip: unknown) => Promise<unknown>;
  state?: () => { sessionId: string | null };
};

test('route planning and drive controls work end-to-end', async ({ page, request }) => {
  const trip = await findDrivableTrip(request);

  await openLoadbotPage(page);
  await page.evaluate(async (currentTrip) => {
    const bridge = (window as unknown as { __loadbot?: LoadbotPlanBridge }).__loadbot;
    if (!bridge?.planTrip) {
      throw new Error('loadbot planTrip bridge was not available');
    }
    return bridge.planTrip({
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
    const bridge = (window as unknown as { __loadbot?: LoadbotPlanBridge }).__loadbot;
    return Boolean(bridge?.state?.().sessionId);
  });

  await page.click('#stop-drive-btn');
  await expect(page.locator('#driving-hud')).toBeHidden();
  await expect(page.locator('#route-panel')).toBeVisible();
});
