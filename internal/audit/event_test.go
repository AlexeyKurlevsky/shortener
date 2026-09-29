package audit

import (
	"encoding/json"
	"testing"
)

func TestEvent_JSON(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		wantJSON string
	}{
		{
			name: "with user id",
			event: Event{
				TS:     12345678,
				Action: ActionShorten,
				UserID: "12315134",
				URL:    "https://example.com",
			},
			wantJSON: `{"ts":12345678,"action":"shorten","user_id":"12315134","url":"https://example.com"}`,
		},
		{
			name: "without user id (omitempty)",
			event: Event{
				TS:     12345678,
				Action: ActionFollow,
				URL:    "https://example.com",
			},
			wantJSON: `{"ts":12345678,"action":"follow","url":"https://example.com"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("got %s, want %s", got, tt.wantJSON)
			}
		})
	}
}

func TestAction_Constants(t *testing.T) {
	if ActionShorten != "shorten" {
		t.Errorf("ActionShorten = %q, want %q", ActionShorten, "shorten")
	}
	if ActionFollow != "follow" {
		t.Errorf("ActionFollow = %q, want %q", ActionFollow, "follow")
	}
}
