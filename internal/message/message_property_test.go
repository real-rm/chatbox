package message

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket
// Property: Message parsing round trip preserves all fields
// **Validates: Requirements 8.1, 8.2, 8.3**
func TestProperty_MessageParsingRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("marshaling then unmarshaling preserves message", prop.ForAll(
		func(msg *Message) bool {
			// Marshal to JSON
			data, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			// Unmarshal back
			var parsed Message
			err = json.Unmarshal(data, &parsed)
			if err != nil {
				return false
			}

			// Compare all fields
			if msg.Type != parsed.Type {
				return false
			}
			if msg.SessionID != parsed.SessionID {
				return false
			}
			if msg.Content != parsed.Content {
				return false
			}
			if msg.FileID != parsed.FileID {
				return false
			}
			if msg.FileURL != parsed.FileURL {
				return false
			}
			if msg.ModelID != parsed.ModelID {
				return false
			}
			if msg.Sender != parsed.Sender {
				return false
			}
			
			// Compare timestamps (truncate to second for JSON precision)
			if !msg.Timestamp.Truncate(time.Second).Equal(parsed.Timestamp.Truncate(time.Second)) {
				return false
			}

			// Compare metadata
			if len(msg.Metadata) != len(parsed.Metadata) {
				return false
			}
			for k, v := range msg.Metadata {
				if parsed.Metadata[k] != v {
					return false
				}
			}

			// Compare error info
			if msg.Error != nil {
				if parsed.Error == nil {
					return false
				}
				if msg.Error.Code != parsed.Error.Code ||
					msg.Error.Message != parsed.Error.Message ||
					msg.Error.Recoverable != parsed.Error.Recoverable ||
					msg.Error.RetryAfter != parsed.Error.RetryAfter {
					return false
				}
			} else if parsed.Error != nil {
				return false
			}

			return true
		},
		genMessage(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property: All messages must have type and sender fields
// **Validates: Requirements 8.2**
func TestProperty_MessageRequiredFields(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("all messages have type and sender", prop.ForAll(
		func(msg *Message) bool {
			// Marshal to JSON
			data, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			// Parse as map to check fields
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			if err != nil {
				return false
			}

			// Type field must be present
			if _, ok := result["type"]; !ok {
				return false
			}

			// Sender field must be present
			if _, ok := result["sender"]; !ok {
				return false
			}

			// Timestamp field must be present
			if _, ok := result["timestamp"]; !ok {
				return false
			}

			return true
		},
		genMessage(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property: JSON output is always valid
// **Validates: Requirements 8.1**
func TestProperty_JSONValidity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("marshaled messages are valid JSON", prop.ForAll(
		func(msg *Message) bool {
			// Marshal to JSON
			data, err := json.Marshal(msg)
			if err != nil {
				return false
			}

			// Verify it's valid JSON by unmarshaling to interface{}
			var result interface{}
			err = json.Unmarshal(data, &result)
			return err == nil
		},
		genMessage(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// genMessage generates random Message instances for property testing
func genMessage() gopter.Gen {
	return gopter.CombineGens(
		genMessageType(),
		gen.Identifier(),      // SessionID - use Identifier for non-empty strings
		gen.AlphaString(),     // Content
		gen.Identifier(),      // FileID
		gen.AlphaString(),     // FileURL
		gen.Identifier(),      // ModelID
		genTime(),
		genSenderType(),
		genMetadata(),
		genErrorInfo(),
	).Map(func(values []interface{}) *Message {
		return &Message{
			Type:      values[0].(MessageType),
			SessionID: values[1].(string),
			Content:   values[2].(string),
			FileID:    values[3].(string),
			FileURL:   values[4].(string),
			ModelID:   values[5].(string),
			Timestamp: values[6].(time.Time),
			Sender:    values[7].(SenderType),
			Metadata:  values[8].(map[string]string),
			Error:     values[9].(*ErrorInfo),
		}
	})
}

// genMessageType generates random MessageType values
func genMessageType() gopter.Gen {
	return gen.OneConstOf(
		TypeUserMessage,
		TypeAIResponse,
		TypeFileUpload,
		TypeVoiceMessage,
		TypeError,
		TypeConnectionStatus,
		TypeTypingIndicator,
		TypeHelpRequest,
		TypeAdminJoin,
		TypeAdminLeave,
		TypeModelSelect,
		TypeLoading,
	)
}

// genSenderType generates random SenderType values
func genSenderType() gopter.Gen {
	return gen.OneConstOf(
		SenderUser,
		SenderAI,
		SenderAdmin,
	)
}

// genTime generates random time values
func genTime() gopter.Gen {
	return gen.Int64Range(0, time.Now().Unix()).Map(func(ts int64) time.Time {
		return time.Unix(ts, 0).UTC()
	})
}

// genMetadata generates random metadata maps (simple version)
func genMetadata() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(map[string]string(nil)),
		gen.Const(map[string]string{"key1": "value1"}),
		gen.Const(map[string]string{"key1": "value1", "key2": "value2"}),
		gen.Const(map[string]string{"filename": "test.pdf", "size": "1024"}),
	)
}

// genErrorInfo generates random ErrorInfo values (can be nil)
func genErrorInfo() gopter.Gen {
	return gen.OneGenOf(
		gen.Const((*ErrorInfo)(nil)),
		gopter.CombineGens(
			gen.Identifier(),  // Code - use Identifier for non-empty
			gen.AlphaString(), // Message
			gen.Bool(),
			gen.IntRange(0, 10000),
		).Map(func(values []interface{}) *ErrorInfo {
			return &ErrorInfo{
				Code:        values[0].(string),
				Message:     values[1].(string),
				Recoverable: values[2].(bool),
				RetryAfter:  values[3].(int),
			}
		}),
	)
}
