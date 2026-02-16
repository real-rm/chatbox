package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageFraming_SeparateFrames verifies that each message is sent as a separate WebSocket frame
// This test addresses requirement 8.1: WebSocket Message Framing
func TestMessageFraming_SeparateFrames(t *testing.T) {
	// Track received frames
	receivedFrames := make([]string, 0)
	frameReceived := make(chan bool, 10)

	// Create a test server that reads individual frames
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Read each frame separately
		for i := 0; i < 3; i++ {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			
			// Verify it's a text message
			assert.Equal(t, websocket.TextMessage, messageType)
			
			// Store the frame data
			receivedFrames = append(receivedFrames, string(data))
			frameReceived <- true
		}
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Start writePump
	go connection.writePump()

	// Create three distinct JSON messages
	msg1 := &message.Message{
		Type:    message.TypeUserMessage,
		Content: "First message",
		Sender:  message.SenderUser,
	}
	msg2 := &message.Message{
		Type:    message.TypeAIResponse,
		Content: "Second message",
		Sender:  message.SenderAI,
	}
	msg3 := &message.Message{
		Type:    message.TypeNotification,
		Content: "Third message",
		Sender:  message.SenderSystem,
	}

	// Marshal messages
	data1, err := json.Marshal(msg1)
	require.NoError(t, err)
	data2, err := json.Marshal(msg2)
	require.NoError(t, err)
	data3, err := json.Marshal(msg3)
	require.NoError(t, err)

	// Send messages to the send channel
	connection.send <- data1
	connection.send <- data2
	connection.send <- data3

	// Wait for all frames to be received
	for i := 0; i < 3; i++ {
		select {
		case <-frameReceived:
			// Frame received
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for frame")
		}
	}

	// Verify we received exactly 3 frames
	assert.Equal(t, 3, len(receivedFrames), "Should receive exactly 3 separate frames")

	// Verify each frame contains valid JSON
	for i, frame := range receivedFrames {
		var msg message.Message
		err := json.Unmarshal([]byte(frame), &msg)
		assert.NoError(t, err, "Frame %d should be valid JSON", i+1)
	}

	// Verify no frames contain concatenated JSON (no newlines between messages)
	for i, frame := range receivedFrames {
		// Count opening braces - should be exactly 1 per frame
		openBraces := strings.Count(frame, "{")
		assert.Equal(t, 1, openBraces, "Frame %d should contain exactly one JSON object", i+1)
	}

	// Verify the content of each frame matches the sent messages
	var parsedMsg1, parsedMsg2, parsedMsg3 message.Message
	require.NoError(t, json.Unmarshal([]byte(receivedFrames[0]), &parsedMsg1))
	require.NoError(t, json.Unmarshal([]byte(receivedFrames[1]), &parsedMsg2))
	require.NoError(t, json.Unmarshal([]byte(receivedFrames[2]), &parsedMsg3))

	assert.Equal(t, "First message", parsedMsg1.Content)
	assert.Equal(t, "Second message", parsedMsg2.Content)
	assert.Equal(t, "Third message", parsedMsg3.Content)
}

// TestMessageFraming_NoNewlineConcatenation verifies that messages are not concatenated with newlines
func TestMessageFraming_NoNewlineConcatenation(t *testing.T) {
	receivedData := make([]byte, 0)
	dataReceived := make(chan bool)

	// Create a test server that reads all data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Read multiple frames
		for i := 0; i < 2; i++ {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			receivedData = append(receivedData, data...)
		}
		dataReceived <- true
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Start writePump
	go connection.writePump()

	// Send two messages
	msg1 := []byte(`{"type":"user_message","content":"Message 1"}`)
	msg2 := []byte(`{"type":"user_message","content":"Message 2"}`)

	connection.send <- msg1
	connection.send <- msg2

	// Wait for data to be received
	select {
	case <-dataReceived:
		// Data received
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for data")
	}

	// Verify the concatenated data does NOT contain newlines between messages
	dataStr := string(receivedData)
	
	// The data should be two separate JSON objects concatenated directly
	// (because we read them into the same buffer), but each was sent in a separate frame
	// We verify that there's no newline character used as a separator
	assert.NotContains(t, dataStr, "}\n{", "Messages should not be separated by newlines")
	assert.NotContains(t, dataStr, "}\r\n{", "Messages should not be separated by CRLF")
}

// TestMessageFraming_RapidMessages verifies that rapid message sending doesn't cause batching
func TestMessageFraming_RapidMessages(t *testing.T) {
	frameCount := 0
	frameCounted := make(chan bool, 10)

	// Create a test server that counts frames
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Read frames for 1 second
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
			frameCount++
			frameCounted <- true
		}
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Start writePump
	go connection.writePump()

	// Send 5 messages rapidly
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		msg := []byte(`{"type":"user_message","content":"Message ` + string(rune('0'+i)) + `"}`)
		connection.send <- msg
	}

	// Wait for all frames to be received
	receivedCount := 0
	for i := 0; i < messageCount; i++ {
		select {
		case <-frameCounted:
			receivedCount++
		case <-time.After(2 * time.Second):
			break
		}
	}

	// Verify we received exactly the number of messages we sent
	assert.Equal(t, messageCount, receivedCount, "Should receive one frame per message")
}
