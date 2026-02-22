package session

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

const defaultTimeout = 15 * time.Minute

// Feature: chat-application-websocket
// Property 51: Help Request State Update
// **Validates: Requirements 16.2, 16.5**
//
// For any help request initiated by a user, the WebSocket_Server should mark
// the session as requiring assistance and persist this state.
func TestProperty_HelpRequestStateUpdate(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("marking help requested updates session state", prop.ForAll(
		func(userID string) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			logger := getTestLogger()
			sm := NewSessionManager(defaultTimeout, logger)

			// Create session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Initially, help should not be requested
			if session.HelpRequested {
				return false
			}

			// Mark help requested
			err = sm.MarkHelpRequested(session.ID)
			if err != nil {
				return false
			}

			// Verify help is now requested
			helpRequested, err := sm.IsHelpRequested(session.ID)
			if err != nil {
				return false
			}

			return helpRequested
		},
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// Feature: chat-application-websocket
// Property 53: Admin Takeover Connection
// **Validates: Requirements 17.3**
//
// For any admin takeover, the WebSocket_Server should establish a connection
// for the Chat_Admin to the user's session.
func TestProperty_AdminTakeoverConnection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("admin takeover marks session with admin info", prop.ForAll(
		func(userID, adminID, adminName string) bool {
			// Skip invalid inputs
			if userID == "" || adminID == "" || adminName == "" {
				return true
			}

			logger := getTestLogger()
			sm := NewSessionManager(defaultTimeout, logger)

			// Create session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Initially, no admin should be assisting
			if session.AssistingAdminID != "" || session.AssistingAdminName != "" {
				return false
			}

			// Mark admin assisted
			err = sm.MarkAdminAssisted(session.ID, adminID, adminName)
			if err != nil {
				return false
			}

			// Verify admin info is stored
			storedAdminID, storedAdminName, err := sm.GetAssistingAdmin(session.ID)
			if err != nil {
				return false
			}

			return storedAdminID == adminID && storedAdminName == adminName
		},
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// Feature: chat-application-websocket
// Property 55: Admin-Assisted Session Marking
// **Validates: Requirements 17.6**
//
// For any session where an admin takeover ends, the session should be marked
// with an admin-assisted flag.
func TestProperty_AdminAssistedSessionMarking(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("admin assistance persists after admin leaves", prop.ForAll(
		func(userID, adminID, adminName string) bool {
			// Skip invalid inputs
			if userID == "" || adminID == "" || adminName == "" {
				return true
			}

			logger := getTestLogger()
			sm := NewSessionManager(defaultTimeout, logger)

			// Create session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Mark admin assisted
			err = sm.MarkAdminAssisted(session.ID, adminID, adminName)
			if err != nil {
				return false
			}

			// Verify admin assisted flag is set
			if !session.AdminAssisted {
				return false
			}

			// Clear admin assistance (admin leaves)
			err = sm.ClearAdminAssistance(session.ID)
			if err != nil {
				return false
			}

			// Admin assisted flag should still be true (historical record)
			// Admin ID and name should be cleared
			storedAdminID, storedAdminName, err := sm.GetAssistingAdmin(session.ID)
			if err != nil {
				return false
			}

			return session.AdminAssisted && storedAdminID == "" && storedAdminName == ""
		},
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// Feature: chat-application-websocket
// Property 57: Admin Session Locking
// **Validates: Requirements 17.8**
//
// For any session being assisted by a Chat_Admin, the session should be marked
// as "assisted by [Admin Name]" and prevent other Chat_Admin users from joining.
func TestProperty_AdminSessionLocking(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("only one admin can assist a session at a time", prop.ForAll(
		func(userID, admin1ID, admin1Name, admin2ID, admin2Name string) bool {
			// Skip invalid inputs
			if userID == "" || admin1ID == "" || admin1Name == "" || admin2ID == "" || admin2Name == "" {
				return true
			}

			// Skip if both admins have the same ID
			if admin1ID == admin2ID {
				return true
			}

			logger := getTestLogger()
			sm := NewSessionManager(defaultTimeout, logger)

			// Create session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// First admin takes over
			err = sm.MarkAdminAssisted(session.ID, admin1ID, admin1Name)
			if err != nil {
				return false
			}

			// Verify first admin is assisting
			storedAdminID, storedAdminName, err := sm.GetAssistingAdmin(session.ID)
			if err != nil {
				return false
			}
			if storedAdminID != admin1ID || storedAdminName != admin1Name {
				return false
			}

			// Second admin tries to take over - should be rejected (atomic check-and-set)
			err = sm.MarkAdminAssisted(session.ID, admin2ID, admin2Name)
			if err == nil {
				return false // should have been rejected
			}

			// Verify first admin is still assisting (not overwritten)
			storedAdminID, storedAdminName, err = sm.GetAssistingAdmin(session.ID)
			if err != nil {
				return false
			}

			return storedAdminID == admin1ID && storedAdminName == admin1Name
		},
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString(),
		gen.Identifier(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
