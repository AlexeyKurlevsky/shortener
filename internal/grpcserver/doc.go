// Package grpcserver реализует gRPC-транспорт для сервиса сокращения
// ссылок. Содержит unary-интерсепторы (recovery, логирование,
// аутентификация) и тонкие хендлеры, делегирующие работу в internal/core
// по аналогии с HTTP-хендлерами из internal/handlers.
package grpcserver
