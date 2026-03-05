// Chat Client Application

// Dev/production configuration via query parameters:
//   ?prefix=/custom/path  — API path prefix (default: /chatbox)
//   ?api=localhost:8080    — API server host:port (default: same as page host)
const _qp = new URLSearchParams(window.location.search);
const PATH_PREFIX = _qp.get("prefix") || "/chatbox";
const API_HOST = _qp.get("api") || window.location.host;

class ChatClient {
  constructor() {
    this.ws = null;
    this.sessionID = null;
    this.userID = null;
    this.isAdminMode = false;
    this.reconnectAttempts = 0;
    this.maxReconnectDelay = 30000; // 30 seconds
    this.reconnectTimer = null;
    this.pingInterval = null;
    this.mediaRecorder = null;
    this.recordingStartTime = null;
    this.recordingTimer = null;
    this.audioChunks = [];

    // Check if in admin mode or shared (read-only) mode
    const urlParams = new URLSearchParams(window.location.search);
    this.isAdminMode = urlParams.get("admin") === "true";
    this.shareToken = urlParams.get("share_token") || null;

    this.initializeElements();
    this.attachEventListeners();

    if (this.shareToken) {
      this.enterReadOnlyMode();
    } else {
      this.connect();
    }
  }

  initializeElements() {
    // Header elements
    this.backBtn = document.getElementById("back-btn");
    this.shareBtn = document.getElementById("share-btn");

    // Status elements
    this.statusText = document.getElementById("status-text");
    this.statusIndicator = document.getElementById("status-indicator");

    // Model selector
    this.modelSelectorContainer = document.getElementById(
      "model-selector-container",
    );
    this.modelSelector = document.getElementById("model-selector");

    // Admin display
    this.adminNameDisplay = document.getElementById("admin-name-display");
    this.adminName = document.getElementById("admin-name");

    // Messages
    this.messagesContainer = document.getElementById("messages-container");
    this._typingIndicator = null;

    // Input elements
    this.messageInput = document.getElementById("message-input");
    this.sendBtn = document.getElementById("send-btn");
    this.fileUploadBtn = document.getElementById("file-upload-btn");
    this.fileInput = document.getElementById("file-input");
    this.cameraBtn = document.getElementById("camera-btn");
    this.voiceBtn = document.getElementById("voice-btn");
    this.helpBtn = document.getElementById("help-btn");

    // Voice recording
    this.voiceRecording = document.getElementById("voice-recording");
    this.recordingTime = document.getElementById("recording-time");
    this.stopRecordingBtn = document.getElementById("stop-recording-btn");

    // Upload progress
    this.uploadProgress = document.getElementById("upload-progress");
    this.progressFill = document.getElementById("progress-fill");
    this.progressText = document.getElementById("progress-text");
  }

  attachEventListeners() {
    // Back button
    this.backBtn.addEventListener("click", () => this.goBackToSessions());

    // Share button
    this.shareBtn.addEventListener("click", () => this.shareSession());

    // Send message
    this.sendBtn.addEventListener("click", () => this.sendMessage());
    this.messageInput.addEventListener("keypress", (e) => {
      if (e.key === "Enter") this.sendMessage();
    });

    // File upload
    this.fileUploadBtn.addEventListener("click", () => this.fileInput.click());
    this.fileInput.addEventListener("change", (e) =>
      this.handleFileUpload(e.target.files[0]),
    );

    // Camera
    this.cameraBtn.addEventListener("click", () => this.handleCamera());

    // Voice recording
    this.voiceBtn.addEventListener("click", () => this.toggleVoiceRecording());
    this.stopRecordingBtn.addEventListener("click", () =>
      this.stopVoiceRecording(),
    );

    // Help request
    this.helpBtn.addEventListener("click", () => this.requestHelp());

    // Model selection
    this.modelSelector.addEventListener("change", (e) =>
      this.selectModel(e.target.value),
    );
  }

  // WebSocket Connection Management
  async connect() {
    const token = this.getJWTToken();
    if (!token) {
      this.updateStatus("disconnected", "No authentication token");
      return;
    }

    this.updateStatus("connecting", "Connecting...");

    // Check for session ID from URL (when navigating from session list)
    let isExistingSession = false;
    if (!this.sessionID) {
      const urlParams = new URLSearchParams(window.location.search);
      const sessionIDFromURL = urlParams.get("session_id");
      const isNewChat = urlParams.get("new") === "1";
      if (sessionIDFromURL) {
        this.sessionID = sessionIDFromURL;
        isExistingSession = true;
      } else if (isNewChat) {
        // Explicit new chat — generate a fresh UUID so the server creates
        // a brand-new session (the active session was already ended).
        this.sessionID = crypto.randomUUID();
      } else {
        // Fetch existing sessions to reuse an active one (server enforces
        // one-active-session-per-user, so creating a new one would fail)
        this.sessionID = await this.resolveSessionID(token);
      }
    }

    // Load message history for existing sessions before connecting
    if (isExistingSession) {
      await this.loadMessageHistory(token);
    }

    this.openWebSocket(token);
  }

  // Fetch the user's sessions and return an active session ID if one exists,
  // otherwise return a new random UUID so the server creates a fresh session.
  async resolveSessionID(token) {
    try {
      const apiBase = _qp.get("api")
        ? `${window.location.protocol}//${_qp.get("api")}`
        : "";
      const response = await fetch(`${apiBase}${PATH_PREFIX}/sessions`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (response.ok) {
        const data = await response.json();
        const sessions = data.sessions || [];
        // Find an active session (is_active=true means no end_time set)
        const active = sessions.find((s) => s.is_active);
        if (active && active.id) {
          return active.id;
        }
      }
    } catch (err) {
      console.warn("Could not fetch existing sessions:", err);
    }
    // No active session found — generate a new ID for server-side creation
    return crypto.randomUUID();
  }

  async loadMessageHistory(token) {
    try {
      const apiBase = _qp.get("api")
        ? `${window.location.protocol}//${_qp.get("api")}`
        : "";
      const response = await fetch(
        `${apiBase}${PATH_PREFIX}/sessions/${this.sessionID}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (!response.ok) {
        console.warn("Failed to load message history:", response.status);
        return;
      }
      const data = await response.json();
      if (data.model_id) {
        this._restoredModelID = data.model_id;
      }
      const messages = data.messages || [];
      for (const msg of messages) {
        this.displayMessage(msg);
      }
    } catch (err) {
      console.warn("Could not load message history:", err);
    }
  }

  openWebSocket(token) {
    // Construct WebSocket URL
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    let wsUrl = `${protocol}//${API_HOST}${PATH_PREFIX}/ws?token=${token}`;

    // Include session ID if reconnecting or loading existing session
    if (this.sessionID) {
      wsUrl += `&session_id=${this.sessionID}`;
    }

    try {
      this.ws = new WebSocket(wsUrl);
      this.ws.onopen = () => this.onOpen();
      this.ws.onmessage = (event) => this.onMessage(event);
      this.ws.onclose = () => this.onClose();
      this.ws.onerror = (error) => this.onError(error);
    } catch (error) {
      console.error("WebSocket connection error:", error);
      this.scheduleReconnect();
    }
  }

  onOpen() {
    console.log("WebSocket connected");
    this.updateStatus("connected", "Connected");
    this.reconnectAttempts = 0;
    this.startHeartbeat();
  }

  onMessage(event) {
    try {
      const message = JSON.parse(event.data);
      console.log("Received message:", message);

      // Keep sessionID in sync with the server-authoritative value.
      // The server may assign a different ID than the client-generated one.
      if (message.session_id && message.session_id !== this.sessionID) {
        this.sessionID = message.session_id;
      }

      switch (message.type) {
        case "connection_status":
          this.handleConnectionStatus(message);
          break;
        case "ai_response":
          this.handleAIResponse(message);
          break;
        case "admin_join":
          this.handleAdminJoin(message);
          break;
        case "admin_leave":
          this.handleAdminLeave(message);
          break;
        case "loading":
          this.handleLoading(message);
          break;
        case "error":
          this.handleError(message);
          break;
        case "file_upload":
          this.handleFileMessage(message);
          break;
        case "voice_message":
          this.handleVoiceMessage(message);
          break;
        case "user_message":
          // Echo user message (if server sends it back)
          this.displayMessage(message);
          break;
        default:
          console.warn("Unknown message type:", message.type);
      }
    } catch (error) {
      console.error("Error parsing message:", error);
    }
  }

  onClose() {
    console.log("WebSocket closed");
    this.updateStatus("disconnected", "Disconnected");
    this.stopHeartbeat();
    this.scheduleReconnect();
  }

  onError(error) {
    console.error("WebSocket error:", error);
  }

  scheduleReconnect() {
    if (this.reconnectTimer) return;

    // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
    const delay = Math.min(
      1000 * Math.pow(2, this.reconnectAttempts),
      this.maxReconnectDelay,
    );

    this.reconnectAttempts++;
    this.updateStatus(
      "connecting",
      `Reconnecting in ${Math.ceil(delay / 1000)}s...`,
    );

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  // Heartbeat — the server sends WebSocket protocol-level pings (writePump);
  // the browser responds with pongs automatically. No application-level ping needed.
  startHeartbeat() {
    // no-op: handled by server-side ping/pong frames
  }

  stopHeartbeat() {
    // no-op
  }

  // Message Handlers
  handleConnectionStatus(message) {
    if (message.session_id) {
      this.sessionID = message.session_id;
    }
    if (message.user_id) {
      this.userID = message.user_id;
    }
    if (message.models && message.models.length > 1) {
      this.showModelSelector(message.models);
    }
  }

  handleAIResponse(message) {
    const isStreaming =
      message.metadata && message.metadata.streaming === "true";
    const isDone = message.metadata && message.metadata.done === "true";

    if (isStreaming) {
      // Only create the bubble when there is actual content to display.
      // Empty chunks (e.g. done=true with no content) should not spawn
      // a visible bubble or hide the typing indicator prematurely.
      if (message.content) {
        if (!this._streamingDiv) {
          this.hideLoading();
          this._streamingDiv = document.createElement("div");
          this._streamingDiv.className = "message ai";

          const contentDiv = document.createElement("div");
          contentDiv.className = "message-content";
          this._streamingDiv.appendChild(contentDiv);

          const timestamp = document.createElement("div");
          timestamp.className = "message-timestamp";
          this._streamingDiv.appendChild(timestamp);

          this.messagesContainer.appendChild(this._streamingDiv);
        }

        const contentEl = this._streamingDiv.querySelector(".message-content");
        contentEl.textContent += message.content;

        const tsEl = this._streamingDiv.querySelector(".message-timestamp");
        tsEl.textContent = this.formatTimestamp(message.timestamp);

        this.scrollToBottom();
      }

      // When the stream is done, release the reference so the next
      // response starts a fresh bubble.
      if (isDone) {
        this.hideLoading();
        this._streamingDiv = null;
      }
    } else {
      // Non-streaming (complete) response — display as a single message
      this.displayMessage(message);
    }
  }

  handleAdminJoin(message) {
    const adminName = message.metadata?.admin_name || "Admin";
    this.showAdminName(adminName);
    this.displaySystemMessage(`${adminName} has joined the conversation`);

    // If we are the admin joining, update UI
    if (this.isAdminMode) {
      this.displaySystemMessage("You have joined as admin");
    }
  }

  handleAdminLeave(message) {
    this.hideAdminName();
    this.displaySystemMessage("Admin has left the conversation");

    // If we are the admin leaving, redirect back to admin dashboard
    if (this.isAdminMode) {
      setTimeout(() => {
        window.close(); // Close the admin takeover window
      }, 2000);
    }
  }

  handleLoading() {
    this.showLoading();
  }

  handleError(message) {
    const errorMsg = message.error?.message || "An error occurred";
    this.displaySystemMessage(`Error: ${errorMsg}`, "error");
    this.hideLoading();
  }

  handleFileMessage(message) {
    this.displayMessage(message);
  }

  handleVoiceMessage(message) {
    this.displayMessage(message);
  }

  // Message Display
  displayMessage(message) {
    const messageDiv = document.createElement("div");
    messageDiv.className = `message ${message.sender}`;

    // Message content
    if (message.content) {
      const contentDiv = document.createElement("div");
      contentDiv.className = "message-content";
      contentDiv.textContent = message.content;
      messageDiv.appendChild(contentDiv);
    }

    // File attachment
    if (message.file_url) {
      const fileDiv = document.createElement("div");
      fileDiv.className = "message-file";
      const fileLink = document.createElement("a");
      fileLink.href = message.file_url;
      fileLink.target = "_blank";
      fileLink.textContent = "📎 View file";
      fileDiv.appendChild(fileLink);
      messageDiv.appendChild(fileDiv);
    }

    // Voice message
    if (message.type === "voice_message" && message.file_url) {
      const audioDiv = document.createElement("div");
      audioDiv.className = "message-audio";
      const audio = document.createElement("audio");
      audio.controls = true;
      audio.src = message.file_url;
      audioDiv.appendChild(audio);
      messageDiv.appendChild(audioDiv);
    }

    // Timestamp
    const timestamp = document.createElement("div");
    timestamp.className = "message-timestamp";
    timestamp.textContent = this.formatTimestamp(message.timestamp);
    messageDiv.appendChild(timestamp);

    this.messagesContainer.appendChild(messageDiv);
    this.scrollToBottom();
  }

  displaySystemMessage(text, type = "info") {
    const messageDiv = document.createElement("div");
    messageDiv.className = "message system";
    messageDiv.style.alignSelf = "center";
    messageDiv.style.background = type === "error" ? "#f8d7da" : "#d1ecf1";
    messageDiv.style.color = type === "error" ? "#721c24" : "#0c5460";
    messageDiv.style.fontSize = "13px";
    messageDiv.style.padding = "6px 12px";
    messageDiv.textContent = text;

    this.messagesContainer.appendChild(messageDiv);
    this.scrollToBottom();
  }

  // Send Message
  sendMessage() {
    const content = this.messageInput.value.trim();
    if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }

    // Ensure any previous streaming bubble is finalized before the next
    // response starts, in case the server's done=true was lost.
    this._streamingDiv = null;

    const message = {
      type: "user_message",
      session_id: this.sessionID,
      content: content,
      timestamp: new Date().toISOString(),
      sender: this.isAdminMode ? "admin" : "user",
    };

    this.ws.send(JSON.stringify(message));
    this.displayMessage(message);
    this.messageInput.value = "";
    this.showLoading();
  }

  // File Upload
  async handleFileUpload(file) {
    if (!file) return;

    this.showUploadProgress(0);

    try {
      const result = await this.uploadFile(file);

      const message = {
        type: "file_upload",
        session_id: this.sessionID,
        file_id: result.file_id,
        file_url: result.file_url,
        timestamp: new Date().toISOString(),
        sender: "user",
        metadata: {
          filename: file.name,
          size: file.size,
          type: file.type,
        },
      };

      this.ws.send(JSON.stringify(message));
      this.displayMessage(message);
      this.hideUploadProgress();
    } catch (error) {
      console.error("File upload error:", error);
      this.displaySystemMessage("File upload failed", "error");
      this.hideUploadProgress();
    }

    // Reset file input
    this.fileInput.value = "";
  }

  async uploadFile(file) {
    // Simulate upload progress
    return new Promise((resolve, reject) => {
      const formData = new FormData();
      formData.append("file", file);

      const xhr = new XMLHttpRequest();

      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
          const progress = (e.loaded / e.total) * 100;
          this.showUploadProgress(progress);
        }
      });

      xhr.addEventListener("load", () => {
        if (xhr.status === 200) {
          try {
            const response = JSON.parse(xhr.responseText);
            resolve(response);
          } catch (error) {
            reject(error);
          }
        } else {
          reject(new Error(`Upload failed: ${xhr.status}`));
        }
      });

      xhr.addEventListener("error", () => {
        reject(new Error("Upload failed"));
      });

      xhr.open("POST", "/api/upload");
      xhr.send(formData);
    });
  }

  // Camera Access
  async handleCamera() {
    try {
      // Check if running in mobile webview
      if (this.fileInput.capture !== undefined) {
        // Use file input with camera capture
        this.fileInput.setAttribute("capture", "environment");
        this.fileInput.click();
      } else {
        // Fallback to regular file selection
        this.fileInput.click();
      }
    } catch (error) {
      console.error("Camera access error:", error);
      this.displaySystemMessage("Camera access failed", "error");
    }
  }

  // Voice Recording
  async toggleVoiceRecording() {
    if (this.mediaRecorder && this.mediaRecorder.state === "recording") {
      this.stopVoiceRecording();
    } else {
      await this.startVoiceRecording();
    }
  }

  async startVoiceRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      this.mediaRecorder = new MediaRecorder(stream);
      this.audioChunks = [];

      this.mediaRecorder.addEventListener("dataavailable", (event) => {
        this.audioChunks.push(event.data);
      });

      this.mediaRecorder.addEventListener("stop", () => {
        const audioBlob = new Blob(this.audioChunks, { type: "audio/webm" });
        this.handleVoiceUpload(audioBlob);

        // Stop all tracks
        stream.getTracks().forEach((track) => track.stop());
      });

      this.mediaRecorder.start();
      this.recordingStartTime = Date.now();
      this.showVoiceRecording();
      this.voiceBtn.classList.add("voice-recording");

      // Update recording time
      this.recordingTimer = setInterval(() => {
        const elapsed = Math.floor(
          (Date.now() - this.recordingStartTime) / 1000,
        );
        const minutes = Math.floor(elapsed / 60);
        const seconds = elapsed % 60;
        this.recordingTime.textContent = `${minutes}:${seconds.toString().padStart(2, "0")}`;
      }, 1000);
    } catch (error) {
      console.error("Voice recording error:", error);
      this.displaySystemMessage("Microphone access denied", "error");
    }
  }

  stopVoiceRecording() {
    if (this.mediaRecorder && this.mediaRecorder.state === "recording") {
      this.mediaRecorder.stop();
      this.hideVoiceRecording();
      this.voiceBtn.classList.remove("voice-recording");

      if (this.recordingTimer) {
        clearInterval(this.recordingTimer);
        this.recordingTimer = null;
      }
    }
  }

  async handleVoiceUpload(audioBlob) {
    this.showUploadProgress(0);

    try {
      const file = new File([audioBlob], `voice_${Date.now()}.webm`, {
        type: "audio/webm",
      });
      const result = await this.uploadFile(file);

      const message = {
        type: "voice_message",
        session_id: this.sessionID,
        file_id: result.file_id,
        file_url: result.file_url,
        timestamp: new Date().toISOString(),
        sender: "user",
      };

      this.ws.send(JSON.stringify(message));
      this.displayMessage(message);
      this.hideUploadProgress();
    } catch (error) {
      console.error("Voice upload error:", error);
      this.displaySystemMessage("Voice upload failed", "error");
      this.hideUploadProgress();
    }
  }

  // Model Selection
  selectModel(modelID) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;

    const message = {
      type: "model_select",
      session_id: this.sessionID,
      model_id: modelID,
      timestamp: new Date().toISOString(),
    };

    this.ws.send(JSON.stringify(message));
    this.modelSelector.value = modelID;
    this.displaySystemMessage(`Switched to ${modelID}`);
  }

  showModelSelector(models) {
    const savedValue = this.modelSelector.value || this._restoredModelID || "";
    this.modelSelector.innerHTML = "";
    models.forEach((model) => {
      const option = document.createElement("option");
      option.value = model.id;
      option.textContent = model.name;
      this.modelSelector.appendChild(option);
    });
    if (savedValue && models.some((m) => m.id === savedValue)) {
      this.modelSelector.value = savedValue;
    }
    this.modelSelectorContainer.style.display = "block";
  }

  // Admin Display
  showAdminName(name) {
    this.adminName.textContent = name;
    this.adminNameDisplay.style.display = "block";
    this.modelSelectorContainer.style.display = "none";
  }

  hideAdminName() {
    this.adminNameDisplay.style.display = "none";
  }

  // Help Request
  requestHelp() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;

    const message = {
      type: "help_request",
      session_id: this.sessionID,
      timestamp: new Date().toISOString(),
    };

    this.ws.send(JSON.stringify(message));
    this.displaySystemMessage("Help request sent. An admin will join shortly.");
  }

  // UI Updates
  updateStatus(status, text) {
    this.statusText.textContent = text;
    this.statusIndicator.className = `indicator ${status}`;
  }

  showLoading() {
    if (this._typingIndicator) return;
    const el = document.createElement("div");
    el.className = "typing-indicator";
    el.appendChild(document.createElement("span"));
    el.appendChild(document.createElement("span"));
    el.appendChild(document.createElement("span"));
    this._typingIndicator = el;
    this.messagesContainer.appendChild(el);
    this.scrollToBottom();
  }

  hideLoading() {
    if (this._typingIndicator) {
      this._typingIndicator.remove();
      this._typingIndicator = null;
    }
  }

  showUploadProgress(progress) {
    this.uploadProgress.style.display = "block";
    this.progressFill.style.width = `${progress}%`;
    this.progressText.textContent = `Uploading... ${Math.round(progress)}%`;
  }

  hideUploadProgress() {
    this.uploadProgress.style.display = "none";
  }

  showVoiceRecording() {
    this.voiceRecording.style.display = "flex";
    this.recordingTime.textContent = "0:00";
  }

  hideVoiceRecording() {
    this.voiceRecording.style.display = "none";
  }

  scrollToBottom() {
    this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
  }

  formatTimestamp(timestamp) {
    const date = new Date(timestamp);
    const hours = date.getHours().toString().padStart(2, "0");
    const minutes = date.getMinutes().toString().padStart(2, "0");
    return `${hours}:${minutes}`;
  }

  // JWT Token Management
  getJWTToken() {
    // Try to get token from URL parameter
    const urlParams = new URLSearchParams(window.location.search);
    let token = urlParams.get("token");

    if (token) {
      // Store token for future use
      sessionStorage.setItem("jwt_token", token);
      return token;
    }

    // Try to get token from session storage
    token = sessionStorage.getItem("jwt_token");
    if (token) {
      return token;
    }

    // Try to get token from parent window (iframe/webview)
    if (window.parent !== window) {
      try {
        window.parent.postMessage({ type: "request_token" }, "*");
      } catch (error) {
        console.error("Failed to request token from parent:", error);
      }
    }

    return null;
  }

  refreshToken() {
    // Request token refresh from parent application
    if (window.parent !== window) {
      try {
        window.parent.postMessage({ type: "refresh_token" }, "*");
      } catch (error) {
        console.error("Failed to request token refresh:", error);
      }
    }
  }

  // Share session — POST to get/create share token, copy link to clipboard
  async shareSession() {
    const hasMessages = this.messagesContainer.querySelector(".message");
    if (!this.sessionID || !hasMessages) {
      this.displaySystemMessage(
        "Start a conversation first — there's nothing to share yet",
        "info",
      );
      return;
    }

    const token = this.getJWTToken();
    if (!token) return;

    try {
      const apiBase = _qp.get("api")
        ? `${window.location.protocol}//${_qp.get("api")}`
        : "";
      const response = await fetch(
        `${apiBase}${PATH_PREFIX}/sessions/${this.sessionID}/share`,
        {
          method: "POST",
          headers: { Authorization: `Bearer ${token}` },
        },
      );

      if (!response.ok) {
        this.displaySystemMessage("Failed to share session", "error");
        return;
      }

      const data = await response.json();

      // Preserve prefix/api params so the shared URL reaches the right server
      const shareParams = new URLSearchParams();
      shareParams.set("share_token", data.share_token);
      const prefix = _qp.get("prefix");
      if (prefix) shareParams.set("prefix", prefix);
      const api = _qp.get("api");
      if (api) shareParams.set("api", api);
      const shareUrl = `${window.location.origin}${window.location.pathname}?${shareParams}`;

      this.showShareDialog(shareUrl);
    } catch (err) {
      console.error("Share error:", err);
      this.displaySystemMessage("Failed to share session", "error");
    }
  }

  showShareDialog(url) {
    // Backdrop
    const backdrop = document.createElement("div");
    backdrop.className = "share-dialog-backdrop";

    // Dialog
    const dialog = document.createElement("div");
    dialog.className = "share-dialog";

    const title = document.createElement("div");
    title.className = "share-dialog-title";
    title.textContent = "Share link";

    const urlInput = document.createElement("input");
    urlInput.type = "text";
    urlInput.value = url;
    urlInput.readOnly = true;
    urlInput.className = "share-dialog-url";
    urlInput.addEventListener("click", () => urlInput.select());

    const actions = document.createElement("div");
    actions.className = "share-dialog-actions";

    const copyBtn = document.createElement("button");
    copyBtn.textContent = "Copy";
    copyBtn.className = "share-dialog-copy";
    copyBtn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(url);
      } catch (_) {
        urlInput.select();
        document.execCommand("copy");
      }
      copyBtn.textContent = "Copied!";
      setTimeout(() => {
        copyBtn.textContent = "Copy";
      }, 1500);
    });

    const closeBtn = document.createElement("button");
    closeBtn.textContent = "Close";
    closeBtn.className = "share-dialog-close";
    const close = () => document.body.removeChild(backdrop);
    closeBtn.addEventListener("click", close);
    backdrop.addEventListener("click", (e) => {
      if (e.target === backdrop) close();
    });

    actions.appendChild(copyBtn);
    actions.appendChild(closeBtn);
    dialog.appendChild(title);
    dialog.appendChild(urlInput);
    dialog.appendChild(actions);
    backdrop.appendChild(dialog);
    document.body.appendChild(backdrop);

    urlInput.select();
  }

  // Read-only mode for shared sessions (no auth, no WebSocket)
  async enterReadOnlyMode() {
    // Hide interactive elements
    document.getElementById("input-area").style.display = "none";
    this.backBtn.style.display = "none";
    this.shareBtn.style.display = "none";
    if (this.modelSelectorContainer) {
      this.modelSelectorContainer.style.display = "none";
    }

    this.updateStatus("connected", "Shared conversation (read-only)");

    // Fetch messages via public endpoint
    try {
      const apiBase = _qp.get("api")
        ? `${window.location.protocol}//${_qp.get("api")}`
        : "";
      const response = await fetch(
        `${apiBase}${PATH_PREFIX}/shared/${this.shareToken}`,
      );

      if (!response.ok) {
        this.updateStatus("disconnected", "Shared session not found");
        return;
      }

      const data = await response.json();
      const messages = data.messages || [];
      for (const msg of messages) {
        this.displayMessage(msg);
      }
    } catch (err) {
      console.error("Failed to load shared session:", err);
      this.updateStatus("disconnected", "Failed to load shared session");
    }
  }

  goBackToSessions() {
    // Close WebSocket cleanly before navigating to avoid
    // "WebSocket is closed due to suspension" console errors.
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.onclose = null; // prevent reconnect logic
      this.ws.close();
      this.ws = null;
    }

    // If in admin mode, close window instead of navigating
    if (this.isAdminMode) {
      window.close();
      return;
    }

    // Navigate back to session list
    const token = this.getJWTToken();
    const apiParam = _qp.get("api") ? `&api=${_qp.get("api")}` : "";
    const sessionsUrl = `sessions.html?token=${encodeURIComponent(token)}${apiParam}`;
    window.location.href = sessionsUrl;
  }
}

// Listen for token from parent window with origin validation
window.addEventListener("message", (event) => {
  // Validate origin to prevent cross-origin token injection
  if (event.origin !== window.location.origin) {
    console.warn("Rejected postMessage from unexpected origin:", event.origin);
    return;
  }
  if (event.data.type === "token") {
    sessionStorage.setItem("jwt_token", event.data.token);
    // Reconnect if needed
    if (
      chatClient &&
      (!chatClient.ws || chatClient.ws.readyState !== WebSocket.OPEN)
    ) {
      chatClient.connect();
    }
  }
});

// Initialize chat client when DOM is ready
let chatClient;
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", () => {
    chatClient = new ChatClient();
  });
} else {
  chatClient = new ChatClient();
}
