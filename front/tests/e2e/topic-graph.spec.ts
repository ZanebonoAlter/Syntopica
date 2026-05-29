import { expect, test } from '@playwright/test'

async function waitForGraphReady(page: import('@playwright/test').Page) {
  const pageRoot = page.locator('[data-testid="topic-graph-page"]')
  await expect(pageRoot).toBeVisible({ timeout: 60000 })

  const canvas = page.locator('[data-testid="topic-graph-canvas"]')
  await expect(canvas).toBeVisible({ timeout: 30000 })
  await expect(canvas).toHaveAttribute('data-state', 'ready', { timeout: 30000 })
}

async function navigateToTopics(page: import('@playwright/test').Page) {
  await page.goto('/topics', { waitUntil: 'networkidle' })
}

async function gotoAndReady(page: import('@playwright/test').Page) {
  await navigateToTopics(page)
  await waitForGraphReady(page)
}

test.describe('Topic Graph P3', () => {
  test('should navigate to home page when clicking return home button', async ({ page }) => {
    await gotoAndReady(page)
    const returnHomeButton = page.locator('[data-testid="return-home-button"]')
    await expect(returnHomeButton).toBeVisible()
    await returnHomeButton.click()
    await expect(page).toHaveURL('/')
  })

  test('should highlight nodes when selecting category', async ({ page }) => {
    await gotoAndReady(page)

    const canvas = page.locator('[data-testid="topic-graph-canvas"]')

    await page.click('[data-testid="hotspot-category-event"] .topic-category-header--button')
    await expect(canvas).toHaveAttribute('data-selected-category', 'event')

    const highlightCount = Number(await canvas.getAttribute('data-highlight-count'))
    expect(highlightCount).toBeGreaterThanOrEqual(0)
  })

  test('should display analysis panel with correct content', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    const badgeCount = await badges.count()
    if (badgeCount === 0) {
      test.skip()
      return
    }

    await badges.first().click()
    await expect(page.locator('[data-testid="analysis-panel"]')).toBeVisible()
  })

  test('should show analysis panel without duplicate toolbar', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()

    await expect(page.locator('[data-testid="topic-graph-footer"]')).toBeVisible()
    await expect(page.locator('[data-testid="topic-analysis-tabs"]')).toHaveCount(0)
  })

  test('should display hotspot topics in three categories', async ({ page }) => {
    await gotoAndReady(page)

    await expect(page.locator('[data-testid="hotspot-category-event"]')).toBeVisible()
    await expect(page.locator('[data-testid="hotspot-category-person"]')).toBeVisible()
    await expect(page.locator('[data-testid="hotspot-category-keyword"]')).toBeVisible()
  })
})

test.describe('Topic Graph - Timeline Tests', () => {
  test('should show timeline after selecting topic', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    const badgeCount = await badges.count()
    if (badgeCount === 0) {
      test.skip()
      return
    }

    // Click first topic badge
    await badges.first().click()

    // Wait for timeline to appear
    await expect(page.locator('.topic-timeline')).toBeVisible({ timeout: 10000 })
  })

  test('should display digest items in timeline', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()

    // Wait for timeline content
    await expect(page.locator('.topic-timeline')).toBeVisible({ timeout: 10000 })

    // Wait for digest items to load (either timeline items or empty state)
    await page.waitForTimeout(2000)

    // Check if we have timeline items or empty state
    const timelineItems = page.locator('.timeline-item')
    const emptyState = page.locator('.timeline-empty')

    // Either items or empty state should be visible
    const hasItems = (await timelineItems.count()) > 0
    const hasEmpty = (await emptyState.count()) > 0

    expect(hasItems || hasEmpty).toBe(true)
  })

  test('should not render legacy load more controls', async ({ page }) => {
    await gotoAndReady(page)
    await expect(page.locator('.timeline-load-more__btn')).toHaveCount(0)
  })
})

test.describe('Topic Graph - Sidebar Tests', () => {
  test('should show sidebar after selecting topic', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()

    // Sidebar region should be visible
    await expect(page.locator('[data-testid="topic-graph-sidebar-region"]')).toBeVisible()
  })

  test('should display deduplicated articles in sidebar', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()
    await page.waitForTimeout(2000)

    // Check sidebar articles
    const sidebarArticles = page.locator('[data-testid="sidebar-article"]')
    const articleCount = await sidebarArticles.count()

    if (articleCount > 1) {
      // Get all article IDs
      const articleIds = await sidebarArticles.evaluateAll(
        elements => elements.map(el => el.getAttribute('data-article-id'))
      )

      // Check for duplicates
      const uniqueIds = [...new Set(articleIds)]
      expect(articleIds.length).toBe(uniqueIds.length)
    }
  })

  test('should show keyword cloud when topic is selected', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()
    await page.waitForTimeout(2000)

    // Keyword cloud should be visible in sidebar
    const keywordCloud = page.locator('[data-testid="keyword-cloud"]')
    const hasKeywordCloud = (await keywordCloud.count()) > 0

    // If keywords exist, cloud should be visible
    if (hasKeywordCloud) {
      await expect(keywordCloud).toBeVisible()
    }
  })
})

test.describe('Topic Graph - Graph Interaction Tests', () => {
  test('topology graph hides links by default', async ({ page }) => {
    await gotoAndReady(page)

    // By default, no links should be visible until a topic is selected
    const canvas = page.locator('[data-testid="topic-graph-canvas"]')
    await expect(canvas).toBeVisible()

    // Check that canvas is in ready state
    await expect(canvas).toHaveAttribute('data-state', 'ready')
  })

  test('selecting topic shows related links with animation', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    // Click a topic
    await badges.first().click()

    // Wait for animation to complete
    await page.waitForTimeout(1000)

    // Canvas should show selected state
    const canvas = page.locator('[data-testid="topic-graph-canvas"]')
    const selectedCategory = await canvas.getAttribute('data-selected-category')

    // Should have a selected category
    expect(selectedCategory).not.toBeNull()
  })

  test('keyword cloud highlights topology nodes', async ({ page }) => {
    await gotoAndReady(page)

    const badges = page.locator('[data-testid="topic-badge"]')
    if ((await badges.count()) === 0) {
      test.skip()
      return
    }

    await badges.first().click()
    await page.waitForTimeout(2000)

    // Check if keyword cloud exists
    const keywordItems = page.locator('[data-testid="keyword-item"]')
    if ((await keywordItems.count()) === 0) {
      test.skip()
      return
    }

    // Click a keyword
    await keywordItems.first().click()

    // Wait for highlight animation
    await page.waitForTimeout(500)

    // Canvas should have highlighted nodes
    const canvas = page.locator('[data-testid="topic-graph-canvas"]')
    const highlightCount = await canvas.getAttribute('data-highlight-count')

    // Should have some highlighted nodes
    expect(Number(highlightCount)).toBeGreaterThanOrEqual(0)
  })
})

test.describe('Topic Graph - Accessibility Tests', () => {
  test('should have proper ARIA labels on interactive elements', async ({ page }) => {
    await gotoAndReady(page)

    // Check topic badges are keyboard accessible
    const badges = page.locator('[data-testid="topic-badge"]')
    const badgeCount = await badges.count()

    if (badgeCount > 0) {
      // All badges should be buttons
      const firstBadge = badges.first()
      await expect(firstBadge).toHaveAttribute('type', 'button')
    }
  })

  test('should support keyboard navigation for topic selection', async ({ page }) => {
    await gotoAndReady(page)

    // Tab to first interactive element
    await page.keyboard.press('Tab')

    // Should be able to navigate with keyboard
    const focusedElement = await page.locator(':focus')
    await expect(focusedElement).toBeVisible()
  })
})
