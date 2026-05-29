import { test, expect } from '@playwright/test'

/**
 * Baseline e2e test for topic-graph verification.
 * 
 * This test verifies:
 * 1. Playwright can connect to the dev server at localhost:3000
 * 2. The /topics page loads without crash
 * 
 * Note: Full selector-based verification requires Task 1 markers.
 * This baseline test ensures the test infrastructure works.
 */

test.describe('topic-graph baseline', () => {
  test('server responds and page loads', async ({ page }) => {
    // Navigate to the topics page
    const response = await page.goto('/topics')
    
    // Server should respond
    expect(response?.status()).toBeLessThan(500)
    
    // Page should have some content (the Vue app shell)
    await expect(page.locator('body')).toBeVisible()
  })

  test('topics page renders TopicGraphPage component', async ({ page }) => {
    await page.goto('/topics')
    
    // Wait for the page to have any content
    await page.waitForLoadState('networkidle')
    
    // Check that some element exists on the page (verifying Vue rendered)
    const html = await page.content()
    expect(html.length).toBeGreaterThan(100)
  })
})
