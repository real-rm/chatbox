// @ts-check
import { test, expect } from "@playwright/test";
import {
  MOCK_SESSIONS,
  MOCK_EMPTY_SESSIONS,
  TEST_JWT_TOKEN,
} from "../helpers/mock-data.js";

test.describe("Sessions Page", () => {
  test.describe("with sessions", () => {
    test.beforeEach(async ({ page }) => {
      // Mock the sessions API
      await page.route("**/chatbox/sessions", (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_SESSIONS),
        });
      });
    });

    test("renders page title and new chat button", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("h1")).toHaveText("Chat Sessions");
      await expect(page.locator("#new-session-btn")).toBeVisible();
      await expect(page.locator("#new-session-btn")).toHaveText("+ New Chat");
    });

    test("displays session list from API", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      // Wait for sessions to render
      const sessionItems = page.locator(".session-item");
      await expect(sessionItems).toHaveCount(3);
    });

    test("shows session names correctly", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator(".session-name").first()).toBeVisible();

      // Check that named sessions show their names
      const names = await page.locator(".session-name").allTextContents();
      expect(names).toContain("Bug report discussion");
      expect(names).toContain("Feature request");
      // Null name should show 'Untitled Chat'
      expect(names).toContain("Untitled Chat");
    });

    test("shows admin-assisted badge on relevant sessions", async ({
      page,
    }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      const badges = page.locator(".admin-badge");
      await expect(badges).toHaveCount(1);
      await expect(badges.first()).toHaveText("Admin Assisted");
    });

    test("shows message count for each session", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      const messageCounts = page.locator(".session-messages");
      await expect(messageCounts).toHaveCount(3);

      const texts = await messageCounts.allTextContents();
      expect(texts).toContain("12 messages");
      expect(texts).toContain("8 messages");
      expect(texts).toContain("3 messages");
    });

    test("navigates to chat when clicking a session", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      const firstSession = page.locator(".session-item").first();
      await firstSession.click();

      // Should navigate to chat.html with session_id
      await expect(page).toHaveURL(/chat\.html\?.*session_id=/);
    });

    test("navigates to new chat when clicking New Chat button", async ({
      page,
    }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      await page.locator("#new-session-btn").click();

      // Should navigate to chat.html without session_id
      await expect(page).toHaveURL(/chat\.html\?token=/);
    });
  });

  test.describe("empty state", () => {
    test.beforeEach(async ({ page }) => {
      await page.route("**/chatbox/sessions", (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_EMPTY_SESSIONS),
        });
      });
    });

    test("shows empty state when no sessions exist", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#empty-state")).toBeVisible();
      await expect(page.locator("#empty-state")).toContainText(
        "No chat sessions yet",
      );
    });

    test("empty state has a start chat button", async ({ page }) => {
      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      const startButton = page.locator("#empty-state .primary-btn");
      await expect(startButton).toBeVisible();
      await expect(startButton).toContainText("Start Your First Chat");
    });
  });

  test.describe("error handling", () => {
    test("shows error when no token is provided", async ({ page }) => {
      // Don't mock the API - no token means fetch won't happen
      await page.goto("/sessions.html");

      // Should show error about no token
      await expect(page.locator("#connection-status")).toBeVisible();
      await expect(page.locator("#status-text")).toContainText(
        "No authentication token",
      );
    });

    test("shows error when API returns error", async ({ page }) => {
      await page.route("**/chatbox/sessions", (route) => {
        route.fulfill({ status: 500 });
      });

      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#connection-status")).toBeVisible();
      await expect(page.locator("#status-text")).toContainText(
        "Failed to load sessions",
      );
    });
  });

  test.describe("JWT token storage", () => {
    test("stores token from URL in sessionStorage", async ({ page }) => {
      await page.route("**/chatbox/sessions", (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MOCK_EMPTY_SESSIONS),
        });
      });

      await page.goto(`/sessions.html?token=${TEST_JWT_TOKEN}`);

      const storedToken = await page.evaluate(() =>
        sessionStorage.getItem("jwt_token"),
      );
      expect(storedToken).toBe(TEST_JWT_TOKEN);
    });
  });
});
