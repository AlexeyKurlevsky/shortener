# pool

Пакет `pool` предоставляет generic-пул объектов, которые умеют сбрасывать
своё состояние через метод `Reset()`. В основе лежит стандартный
[`sync.Pool`](https://pkg.go.dev/sync#Pool): «тяжёлые» объекты
переиспользуются, а перед возвратом в пул их состояние автоматически
обнуляется.

Пакет хорошо сочетается с кодогенератором
`internal/resetgen`, который создаёт методы
`Reset()` для структур, помеченных комментарием `// generate:reset`.

## Зачем это нужно

`sync.Pool` освобождает от давления на GC и снижает число аллокаций,
но он ничего не знает про состояние объектов. Если вернуть в пул
«грязный» объект, следующий `Get` получит мусор. `Pool[T]`
гарантирует, что объекты в пул попадают только в сброшенном виде.

## API

```go
type Resettable interface {
    Reset()
}

type Pool[T Resettable] struct { /* ... */ }

// New создаёт Pool и настраивает фабрику для создания новых объектов.
// Фабрика вызывается, когда Get() обращается к пустому пулу.
func New[T Resettable](factory func() T) *Pool[T]

// Get возвращает объект из пула (или свежий из фабрики).
func (p *Pool[T]) Get() T

// Put сбрасывает состояние объекта и возвращает его в пул.
func (p *Pool[T]) Put(v T)
```

## Пример

### 1. Объявляем структуру и помечаем её для генератора

```go
//go:generate go run ./cmd/reset .
package buffer

// generate:reset
type Buffer struct {
    Data []byte
    Meta map[string]string
}
```

После запуска `go generate ./...` рядом появится `reset.gen.go`
с методом:

```go
func (b *Buffer) Reset() {
    if b == nil {
        return
    }
    b.Data = b.Data[:0]
    clear(b.Meta)
}
```

Значит, тип `*Buffer` удовлетворяет интерфейсу `pool.Resettable`,
и его можно подставить в `Pool[T]`.

### 2. Используем пул

```go
package main

import (
    "fmt"

    "github.com/AlexeyKurlevsky/shortener/internal/pool"
)

func main() {
    // T = *Buffer — именно указатель реализует Reset().
    p := pool.New(func() *Buffer {
        return &Buffer{}
    })

    b := p.Get()
    b.Data = append(b.Data, "hello"...)
    b.Meta = map[string]string{"k": "v"}
    fmt.Println(len(b.Data), len(b.Meta)) // 5 1

    p.Put(b) // Reset() вызван, объект вернулся в пул

    b2 := p.Get()
    fmt.Println(len(b2.Data), len(b2.Meta)) // 0 0 — состояние сброшено
}
```
