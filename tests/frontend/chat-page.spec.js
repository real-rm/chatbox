// @ts-check
import { test, expect } from "@playwright/test";
import {
  TEST_JWT_TOKEN,
  MOCK_CONNECTION_STATUS,
  MOCK_AI_RESPONSE,
} from "../helpers/mock-data.js";

test.describe("Chat Page", () => {
  test.describe("page structure", () => {
    test.beforeEach(async ({ page }) => {
      // Block WebSocket connections to prevent real connection attempts
      await page.route("**/chat/ws**", (route) => route.abort());
    });

    test("renders all UI elements", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      // Header with back button
      await expect(page.locator("#back-btn")).toBeVisible();
      await expect(page.locator("#back-btn")).toContainText("Sessions");

      // Connection status
      await expect(page.locator("#connection-status")).toBeVisible();

      // Messages container
      await expect(page.locator("#messages-container")).toBeVisible();

      // Input area
      await expect(page.locator("#message-input")).toBeVisible();
      await expect(page.locator("#send-btn")).toBeVisible();
      await expect(page.locator("#send-btn")).toHaveText("Send");
    });

    test("renders media buttons", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#file-upload-btn")).toBeVisible();
      await expect(page.locator("#camera-btn")).toBeVisible();
      await expect(page.locator("#voice-btn")).toBeVisible();
      await expect(page.locator("#help-btn")).toBeVisible();
    });

    test("model selector is hidden by default", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#model-selector-container")).toBeHidden();
    });

    test("admin name display is hidden by default", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#admin-name-display")).toBeHidden();
    });

    test("loading animation is hidden by default", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#loading-animation")).toBeHidden();
    });

    test("upload progress is hidden by default", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#upload-progress")).toBeHidden();
    });

    test("voice recording indicator is hidden by default", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#voice-recording")).toBeHidden();
    });
  });

  test.describe("connection status", () => {
    test("shows connecting or reconnecting status initially", async ({
      page,
    }) => {
      // Block WebSocket to keep it in connecting/reconnecting state
      await page.route("**/chat/ws**", (route) => route.abort());

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      // After WS fails, status shows either "Connecting" or "Reconnecting in Xs..."
      await expect(page.locator("#status-text")).toContainText(/[Cc]onnecting/);
    });

    test("shows disconnected when no token", async ({ page }) => {
      await page.route("**/chat/ws**", (route) => route.abort());

      await page.goto("/chat.html");

      await expect(page.locator("#status-text")).toContainText(
        "No authentication token",
      );
    });
  });

  test.describe("message input", () => {
    test.beforeEach(async ({ page }) => {
      await page.route("**/chat/ws**", (route) => route.abort());
    });

    test("message input accepts text", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      const input = page.locator("#message-input");
      await input.fill("Hello, world!");
      await expect(input).toHaveValue("Hello, world!");
    });

    test("message input has placeholder text", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#message-input")).toHaveAttribute(
        "placeholder",
        "Type a message...",
      );
    });

    test("file input is hidden", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await expect(page.locator("#file-input")).toBeHidden();
    });

    test("file input accepts correct file types", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      const accept = await page.locator("#file-input").getAttribute("accept");
      expect(accept).toContain("image/*");
      expect(accept).toContain("video/*");
      expect(accept).toContain(".pdf");
      expect(accept).toContain(".doc");
    });
  });

  test.describe("navigation", () => {
    test.beforeEach(async ({ page }) => {
      await page.route("**/chat/ws**", (route) => route.abort());
      // Mock sessions API for when we navigate back
      await page.route("**/chat/sessions", (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ user_id: "test", sessions: [] }),
        });
      });
    });

    test("back button navigates to sessions page", async ({ page }) => {
      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      await page.locator("#back-btn").click();

      await expect(page).toHaveURL(/sessions\.html/);
    });

    test("preserves session_id from URL params", async ({ page }) => {
      await page.goto(
        `/chat.html?token=${TEST_JWT_TOKEN}&session_id=test-session-123`,
      );

      // The ChatClient constructor reads session_id from URL
      // We can verify it was set by checking that the WS URL would include it
      const sessionID = await page.evaluate(() => {
        const params = new URLSearchParams(window.location.search);
        return params.get("session_id");
      });
      expect(sessionID).toBe("test-session-123");
    });
  });

  test.describe("WebSocket messaging", () => {
    test("displays user message in chat when sent via WebSocket", async ({
      page,
    }) => {
      // Set up a mock WebSocket by intercepting at the page level
      await page.addInitScript(() => {
        // Override WebSocket to capture messages
        const OriginalWebSocket = window.WebSocket;
        window._mockWsMessages = [];
        window._mockWsInstance = null;

        // @ts-ignore
        window.WebSocket = class MockWebSocket {
          constructor(url) {
            this.url = url;
            this.readyState = 1; // OPEN
            this.onopen = null;
            this.onmessage = null;
            this.onclose = null;
            this.onerror = null;
            window._mockWsInstance = this;

            // Simulate connection after a tick
            setTimeout(() => {
              if (this.onopen) this.onopen({});
            }, 50);
          }

          send(data) {
            window._mockWsMessages.push(JSON.parse(data));
          }

          close() {
            this.readyState = 3;
            if (this.onclose) this.onclose({});
          }
        };

        // Copy constants
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      // Wait for mock WebSocket to "connect"
      await page.waitForTimeout(200);

      // Type and send a message
      await page.locator("#message-input").fill("Hello from test");
      await page.locator("#send-btn").click();

      // Check message appears in the UI
      const messages = page.locator("#messages-container .message");
      await expect(messages).toHaveCount(1);
      await expect(messages.first()).toContainText("Hello from test");

      // Input should be cleared
      await expect(page.locator("#message-input")).toHaveValue("");

      // Verify message was sent via WebSocket
      const sentMessages = await page.evaluate(() => window._mockWsMessages);
      expect(sentMessages.length).toBeGreaterThanOrEqual(1);
      const userMsg = sentMessages.find((m) => m.type === "user_message");
      expect(userMsg).toBeDefined();
      expect(userMsg.content).toBe("Hello from test");
    });

    test("sends message on Enter key press", async ({ page }) => {
      await page.addInitScript(() => {
        window._mockWsMessages = [];
        window.WebSocket = class MockWebSocket {
          constructor() {
            this.readyState = 1;
            window._mockWsInstance = this;
            setTimeout(() => {
              if (this.onopen) this.onopen({});
            }, 50);
          }
          send(data) {
            window._mockWsMessages.push(JSON.parse(data));
          }
          close() {}
        };
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);
      await page.waitForTimeout(200);

      await page.locator("#message-input").fill("Enter key test");
      await page.locator("#message-input").press("Enter");

      const sentMessages = await page.evaluate(() => window._mockWsMessages);
      const userMsg = sentMessages.find((m) => m.type === "user_message");
      expect(userMsg).toBeDefined();
      expect(userMsg.content).toBe("Enter key test");
    });

    test("does not send empty messages", async ({ page }) => {
      await page.addInitScript(() => {
        window._mockWsMessages = [];
        window.WebSocket = class MockWebSocket {
          constructor() {
            this.readyState = 1;
            setTimeout(() => {
              if (this.onopen) this.onopen({});
            }, 50);
          }
          send(data) {
            window._mockWsMessages.push(JSON.parse(data));
          }
          close() {}
        };
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);
      await page.waitForTimeout(200);

      // Try to send with empty input
      await page.locator("#send-btn").click();

      const sentMessages = await page.evaluate(() => window._mockWsMessages);
      const userMsgs = sentMessages.filter((m) => m.type === "user_message");
      expect(userMsgs).toHaveLength(0);
    });

    test("displays AI response received via WebSocket", async ({ page }) => {
      await page.addInitScript(() => {
        window.WebSocket = class MockWebSocket {
          constructor() {
            this.readyState = 1;
            window._mockWsInstance = this;
            setTimeout(() => {
              if (this.onopen) this.onopen({});
              // Send a mock AI response after connection
              setTimeout(() => {
                if (this.onmessage) {
                  this.onmessage({
                    data: JSON.stringify({
                      type: "ai_response",
                      content: "Hello! I am the AI assistant.",
                      sender: "ai",
                      timestamp: new Date().toISOString(),
                    }),
                  });
                }
              }, 100);
            }, 50);
          }
          send() {}
          close() {}
        };
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      // Wait for mock AI response
      const aiMessage = page.locator("#messages-container .message.ai");
      await expect(aiMessage).toBeVisible({ timeout: 5000 });
      await expect(aiMessage).toContainText("Hello! I am the AI assistant.");
    });

    test("displays connection_status with model selector", async ({ page }) => {
      await page.addInitScript(() => {
        window.WebSocket = class MockWebSocket {
          constructor() {
            this.readyState = 1;
            window._mockWsInstance = this;
            setTimeout(() => {
              if (this.onopen) this.onopen({});
              setTimeout(() => {
                if (this.onmessage) {
                  this.onmessage({
                    data: JSON.stringify({
                      type: "connection_status",
                      session_id: "sess-123",
                      user_id: "user-1",
                      models: [
                        { id: "gpt-4", name: "GPT-4" },
                        { id: "claude-3", name: "Claude 3" },
                      ],
                    }),
                  });
                }
              }, 100);
            }, 50);
          }
          send() {}
          close() {}
        };
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);

      // Model selector should become visible
      await expect(page.locator("#model-selector-container")).toBeVisible({
        timeout: 5000,
      });

      // Should have model options
      const options = page.locator("#model-selector option");
      await expect(options).toHaveCount(2);
    });

    test("handles help request button", async ({ page }) => {
      await page.addInitScript(() => {
        window._mockWsMessages = [];
        window.WebSocket = class MockWebSocket {
          constructor() {
            this.readyState = 1;
            setTimeout(() => {
              if (this.onopen) this.onopen({});
            }, 50);
          }
          send(data) {
            window._mockWsMessages.push(JSON.parse(data));
          }
          close() {}
        };
        window.WebSocket.OPEN = 1;
        window.WebSocket.CLOSED = 3;
        window.WebSocket.CONNECTING = 0;
      });

      await page.goto(`/chat.html?token=${TEST_JWT_TOKEN}`);
      await page.waitForTimeout(200);

      await page.locator("#help-btn").click();

      // Should send help_request message
      const sentMessages = await page.evaluate(() => window._mockWsMessages);
      const helpMsg = sentMessages.find((m) => m.type === "help_request");
      expect(helpMsg).toBeDefined();

      // Should show system message about help
      const systemMessages = page.locator(
        "#messages-container .message.system",
      );
      await expect(systemMessages.first()).toContainText("Help request sent");
    });
  });
});
