import { expect, test, type Page } from '@playwright/test'
import { base, signIn } from '../../../../gateway/shell/tests/e2e/helpers'

// Quickstart §4 flow for the paperless remote at the three reference widths.
// Needs a full platform; skips without operator credentials.
const password = process.env.E2E_OPERATOR_PASSWORD ?? ''
const email = process.env.E2E_OPERATOR_EMAIL ?? 'ops@example.org'
const viewports = [{ name: 'phone', width: 320, height: 640 }, { name: 'tablet', width: 768, height: 1024 }, { name: 'desktop', width: 1280, height: 800 }]

async function openNav(page: Page, group: string, entry: string): Promise<void> {
  const burger = page.getByRole('button', { name: 'Open navigation' })
  if (await burger.isVisible()) await burger.click()
  const g = page.getByTestId('nav-group-' + group)
  if ((await g.getAttribute('aria-expanded')) !== 'true') await g.click()
  await page.getByTestId('nav-' + group).filter({ hasText: entry }).first().click()
}

test.describe('paperless remote', () => {
  test.skip(!password, 'E2E_OPERATOR_PASSWORD not set')
  for (const vp of viewports) {
    test(`${vp.name}: upload dialog, document drawer, category tree, search`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height })
      const violations: string[] = []
      await page.addInitScript(() => document.addEventListener('securitypolicyviolation', (e) => console.error('CSP:' + (e as SecurityPolicyViolationEvent).violatedDirective)))
      page.on('console', (m) => { if (m.text().startsWith('CSP:')) violations.push(m.text()) })
      await page.goto(base + '/')
      await signIn(page, email, password)
      await openNav(page, 'paperless', 'Documents')
      await page.getByTestId('doc-upload').click()
      const dialog = page.getByRole('dialog')
      await dialog.getByTestId('upload-submit').click()
      await expect(dialog.getByRole('alert')).toContainText('Choose a file')
      const name = 'e2e-' + vp.name + '-' + Date.now().toString(36)
      await dialog.locator('input[type=file]').setInputFiles({ name: name + '.txt', mimeType: 'text/plain', buffer: Buffer.from('paperless e2e ' + name) })
      await dialog.getByTestId('upload-submit').click()
      await expect(dialog).toBeHidden()
      await expect(page.getByTestId('documents-table')).toContainText(name)
      await page.getByTestId('documents-table').getByText(name).first().click()
      const drawer = page.locator('aside[role=dialog]')
      await expect(drawer).toContainText(name + '.txt')
      await page.keyboard.press('Escape')
      await openNav(page, 'paperless', 'Categories')
      await expect(page.locator('main h1')).toHaveText('Categories')
      await openNav(page, 'paperless', 'Search')
      await page.getByTestId('search-input').locator('input').fill(name)
      await page.getByTestId('search-go').click()
      await expect(page.locator('main')).toContainText(/No matches|rank/)
      await openNav(page, 'paperless', 'Dashboard')
      await expect(page.locator('.stat-tile').first()).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0)
      expect(await page.locator('main [style]').count()).toBe(0)
      expect(violations).toEqual([])
    })
  }
})
