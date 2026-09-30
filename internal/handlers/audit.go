package handlers

import (
	"context"

	"github.com/AlexeyKurlevsky/shortener/internal/audit"
)

type AuditPublisher interface {
	Publish(ctx context.Context, e audit.Event)
}
