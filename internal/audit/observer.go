package audit

import "context"

type Observer interface {
	Notify(ctx context.Context, e Event) error
}
