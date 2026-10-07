package handlers

import (
	"github.com/AlexeyKurlevsky/shortener/internal/audit"
)

type AuditPublisher interface {
	Publish(e audit.Event)
}
