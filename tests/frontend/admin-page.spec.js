// @ts-check
import { test, expect } from "@playwright/test";
import {
  TEST_ADMIN_TOKEN,
  MOCK_ADMIN_SESSIONS,
  MOCK_ADMIN_METRICS,
} from "../helpers/mock-data.js";

test.describe("Admin Dashboard", () => {
  test.beforeEach(async ({ page }) => {
    // Mock admin API endpoints
    await page.route("**/chatbox/admin/metrics**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_ADMIN_METRICS),
      });
    });

    await page.route("**/chatbox/admin/sessions**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_ADMIN_SESSIONS),
      });
    });
  });

  test.describe("page structure", () => {
    test("renders header with title and controls", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator("h1")).toHaveText("Admin Dashboard");
      await expect(page.locator("#refresh-btn")).toBeVisible();
      await expect(page.locator("#auto-refresh-status")).toContainText(
        "Auto-refresh",
      );
    });

    test("renders metrics cards", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator(".metric-card")).toHaveCount(4);
      await expect(page.locator(".metric-label").nth(0)).toHaveText(
        "Active Sessions",
      );
      await expect(page.locator(".metric-label").nth(1)).toHaveText(
        "Total Sessions",
      );
      await expect(page.locator(".metric-label").nth(2)).toHaveText(
        "Avg Concurrent",
      );
      await expect(page.locator(".metric-label").nth(3)).toHaveText(
        "Total Tokens",
      );
    });

    test("renders filter controls", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator("#filter-user-id")).toBeVisible();
      await expect(page.locator("#filter-date-start")).toBeVisible();
      await expect(page.locator("#filter-date-end")).toBeVisible();
      await expect(page.locator("#filter-status")).toBeVisible();
      await expect(page.locator("#filter-admin")).toBeVisible();
      await expect(page.locator("#apply-filters-btn")).toBeVisible();
      await expect(page.locator("#clear-filters-btn")).toBeVisible();
    });

    test("renders sorting controls", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator("#sort-field")).toBeVisible();
      await expect(page.locator("#sort-order")).toBeVisible();
    });

    test("renders sessions table headers", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      const headers = page.locator("#sessions-table thead th");
      await expect(headers).toHaveCount(11);

      const headerTexts = await headers.allTextContents();
      expect(headerTexts).toContain("User ID");
      expect(headerTexts).toContain("Session ID");
      expect(headerTexts).toContain("Status");
      expect(headerTexts).toContain("Tokens");
      expect(headerTexts).toContain("Actions");
    });
  });

  test.describe("metrics display", () => {
    test("displays metrics values from API", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Wait for data to load
      await expect(page.locator("#active-sessions")).not.toHaveText("-");

      await expect(page.locator("#active-sessions")).toHaveText("5");
      await expect(page.locator("#total-sessions")).toHaveText("142");
      await expect(page.locator("#avg-concurrent")).toHaveText("3.2");
      await expect(page.locator("#total-tokens")).toHaveText("580,000");
    });
  });

  test.describe("sessions table", () => {
    test("displays sessions from API", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Wait for table to populate
      const rows = page.locator("#sessions-tbody tr");
      await expect(rows).toHaveCount(2);
    });

    test("shows active/ended status badges", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      const statusBadges = page.locator("#sessions-tbody .status-badge");
      await expect(statusBadges).toHaveCount(2);

      const texts = await statusBadges.allTextContents();
      const trimmed = texts.map((t) => t.trim());
      expect(trimmed).toContain("Active");
      expect(trimmed).toContain("Ended");
    });

    test("shows takeover button only for active sessions", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Only 1 active session should have a takeover button
      const takeoverButtons = page.locator(
        "#sessions-tbody [data-takeover-session]",
      );
      await expect(takeoverButtons).toHaveCount(1);
    });

    test("shows admin badge for admin-assisted sessions", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      const adminBadge = page.locator("#sessions-tbody .admin-badge");
      await expect(adminBadge).toHaveCount(1);
      await expect(adminBadge).toContainText("Admin Jane");
    });

    test("truncates session IDs", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Session IDs should be truncated with "..."
      const cells = page.locator("#sessions-tbody td");
      const allText = await cells.allTextContents();
      const sessionIdCells = allText.filter((t) => t.includes("..."));
      expect(sessionIdCells.length).toBeGreaterThanOrEqual(2);
    });
  });

  test.describe("filters", () => {
    test("applies user ID filter", async ({ page }) => {
      let lastRequestUrl = "";
      await page.route("**/chatbox/admin/sessions**", (route) => {
        lastRequestUrl = route.request().url();
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await page.locator("#filter-user-id").fill("user-100");
      await page.locator("#apply-filters-btn").click();

      // Wait for the request to be made
      await page.waitForTimeout(500);

      expect(lastRequestUrl).toContain("user_id=user-100");
    });

    test("applies status filter", async ({ page }) => {
      let lastRequestUrl = "";
      await page.route("**/chatbox/admin/sessions**", (route) => {
        lastRequestUrl = route.request().url();
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await page.locator("#filter-status").selectOption("active");
      await page.locator("#apply-filters-btn").click();

      await page.waitForTimeout(500);

      expect(lastRequestUrl).toContain("status=active");
    });

    test("clears all filters", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Set some filters
      await page.locator("#filter-user-id").fill("test-user");
      await page.locator("#filter-status").selectOption("active");

      // Clear filters
      await page.locator("#clear-filters-btn").click();

      await expect(page.locator("#filter-user-id")).toHaveValue("");
      await expect(page.locator("#filter-status")).toHaveValue("");
    });
  });

  test.describe("sorting", () => {
    test("includes sort parameters in API request via apply filters", async ({
      page,
    }) => {
      const requestUrls = [];
      await page.route("**/chatbox/admin/sessions**", (route) => {
        requestUrls.push(route.request().url());
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Wait for initial load
      await page.waitForTimeout(500);

      // Change sort field and apply filters (applyFilters updates currentSort)
      await page.locator("#sort-field").selectOption("duration");
      await page.locator("#apply-filters-btn").click();

      // Wait for the new request
      await page.waitForTimeout(500);

      const durationRequest = requestUrls.find((url) =>
        url.includes("sort_by=duration"),
      );
      expect(durationRequest).toBeDefined();
    });
  });

  test.describe("auto-refresh", () => {
    test("auto-refresh is enabled by default", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator("#auto-refresh-status")).toContainText("ON");
    });

    test("toggles auto-refresh on click", async ({ page }) => {
      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await page.locator("#auto-refresh-status").click();

      await expect(page.locator("#auto-refresh-status")).toContainText("OFF");

      await page.locator("#auto-refresh-status").click();

      await expect(page.locator("#auto-refresh-status")).toContainText("ON");
    });
  });

  test.describe("empty state", () => {
    test("shows empty state when no sessions", async ({ page }) => {
      await page.route("**/chatbox/admin/sessions**", (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      await expect(page.locator("#empty-state")).toBeVisible();
      await expect(page.locator("#empty-state")).toContainText(
        "No sessions found",
      );
    });
  });

  test.describe("refresh", () => {
    test("refresh button triggers data reload", async ({ page }) => {
      let requestCount = 0;
      await page.route("**/chatbox/admin/sessions**", (route) => {
        requestCount++;
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_ADMIN_SESSIONS),
        });
      });

      await page.goto(`/admin.html?token=${TEST_ADMIN_TOKEN}`);

      // Wait for initial load
      await page.waitForTimeout(500);
      const initialCount = requestCount;

      // Click refresh
      await page.locator("#refresh-btn").click();

      await page.waitForTimeout(500);

      expect(requestCount).toBeGreaterThan(initialCount);
    });
  });
});
