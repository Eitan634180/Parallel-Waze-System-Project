import { expect, test } from 'playwright/test';

import { testConfig } from '../config/test-config.mjs';
import { findDrivableTrip, openLoadbotPage } from './test-helpers';

type LoadbotDriveBridge = {
  driveTrip?: (trip: unknown) => Promise<unknown>;
  stopDriving?: () => Promise<void>;
  state?: () => { debug?: { last_reroute_reason?: string | null } | null };
};

test('off-route reroute and debug-car updates surface in the UI', async ({ page, request }) => {
  const trip = await findDrivableTrip(request);

  await openLoadbotPage(page);
  await page.evaluate(async (currentTrip) => {
    const bridge = (window as unknown as { __loadbot?: LoadbotDriveBridge }).__loadbot;
    if (!bridge?.driveTrip) {
      throw new Error('loadbot driveTrip bridge was not available');
    }
    return bridge.driveTrip({
      source: currentTrip.source,
      dest: currentTrip.dest,
    });
  }, trip);

  await expect(page.locator('#driving-hud')).toBeVisible();
  await page.click('#debug-toggle-btn');
  await expect(page.locator('#debug-panel')).toBeVisible();

  await expect(page.locator('#debug-main-status')).toContainText('Main car session');
  await page.fill('#debug-car-count', '1');
  await page.click('#debug-add-car-btn');
  await expect(page.locator('#debug-car-status')).toContainText('Active test cars:', { timeout: testConfig.e2e.actionTimeoutMs });
  await expect(page.locator('#debug-car-status')).toContainText(/Active test cars:\s*([1-9]\d*)/, { timeout: testConfig.e2e.actionTimeoutMs });

  await page.click('#debug-drift-btn');
  await expect(page.locator('#alert-title')).toHaveText('Off-route reroute', { timeout: testConfig.e2e.actionTimeoutMs });
  await page.waitForFunction(() => {
    const bridge = (window as unknown as { __loadbot?: LoadbotDriveBridge }).__loadbot;
    return Boolean(bridge?.state?.().debug?.last_reroute_reason);
  }, null, { timeout: testConfig.e2e.actionTimeoutMs });

  await page.evaluate(async () => {
    const bridge = (window as unknown as { __loadbot?: LoadbotDriveBridge }).__loadbot;
    await bridge?.stopDriving?.();
  });
});
