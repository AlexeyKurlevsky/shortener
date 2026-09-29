package audit

type Action string

const (
	ActionShorten Action = "shorten"
	ActionFollow  Action = "follow"
)

type Event struct {
	TS     int64  `json:"ts"`
	Action Action `json:"action"`
	UserID string `json:"user_id,omitempty"`
	URL    string `json:"url"`
}
