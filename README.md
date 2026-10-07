# go-musthave-shortener-tpl

Шаблон репозитория для трека «Сервис сокращения URL».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-shortener-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Профилирование памяти

Я профилировал heap через `pprof` (`/debug/pprof/heap`) и снизил потребление в покое с **~4.9 MB до ~3.7 MB**, а также убрал несколько источников роста под нагрузкой.

### Что я сделал

**Storage**
- Перешёл с `database/sql` + stdlib-обёртки на `pgxpool` — убрал лишний слой prepare/exec и связанные аллокации (`ctxwatch`).
- Ограничил пул соединений
- Вынес миграции `golang-migrate` в отдельную функцию с обязательным `defer m.Close()` — убрал висящую горутину lock.
- Переписал `BatchSave` через `pgx.Batch` — один round-trip на весь батч вместо N отдельных `Exec`.
- Закрываю `*sql.DB` / `pool` во всех ветках ошибок инициализации.

**Logger**
- Отключил sampling (`cfg.Sampling = nil`) — убрал `zapcore.newCounters` из профиля.
- Сделал `Initialize` идемпотентным через `sync.Once` — логгер создаётся один раз.
- Добавил `Sync()` для сброса буферов при shutdown.

### Результат

| Метрика | До | После |
|---|---|---|
| `inuse_space` в покое | 4.9 MB | 3.7 MB |
| `zapcore.newCounters` | 768 kB | 0 |
| `ctxwatch.Watch.func1` | 1024 kB | 0 |
| `migrate.(*Migrate).lock.func2` | 512 kB | 0 |

### Что оставил на будущее

- `compress/flate.NewWriter` (655 kB) — `gzip.Writer` создаётся на каждый запрос, кандидат на `sync.Pool`.
- `handlers.NewHandler` (528 kB) — надо проверить, что вызывается один раз.


```
File: main
Build ID: 681bd648625097cffbb8fd1708857384c3b0c31d
Type: inuse_space
Time: 2026-09-28 22:30:33 MSK
Showing nodes accounting for -1178.75kB, 23.93% of 4925.47kB total
      flat  flat%   sum%        cum   cum%
-1024.11kB 20.79% 20.79% -1024.11kB 20.79%  github.com/jackc/pgconn/internal/ctxwatch.(*ContextWatcher).Watch.func1
 -768.26kB 15.60% 36.39%  -768.26kB 15.60%  go.uber.org/zap/zapcore.newCounters (inline)
  655.29kB 13.30% 23.09%   655.29kB 13.30%  compress/flate.NewWriter (inline)
 -570.04kB 11.57% 34.66%  -570.04kB 11.57%  github.com/AlexeyKurlevsky/shortener/internal/audit.NewPublisher (inline)
  528.17kB 10.72% 23.94%   528.17kB 10.72%  github.com/AlexeyKurlevsky/shortener/internal/handlers.NewHandler
  512.20kB 10.40% 13.54%   512.20kB 10.40%  github.com/jackc/pgx/v5/pgtype.(*Map).planEncodeDepth
 -512.01kB 10.40% 23.93%  -512.01kB 10.40%  runtime.mallocgcSmallScanNoHeaderSC2
         0     0% 23.93%   655.29kB 13.30%  compress/gzip.(*Writer).Close
         0     0% 23.93%   655.29kB 13.30%  compress/gzip.(*Writer).Write
         0     0% 23.93%   512.20kB 10.40%  github.com/AlexeyKurlevsky/shortener/internal/handlers.(*Handler).CreateShortURL
         0     0% 23.93%   512.20kB 10.40%  github.com/AlexeyKurlevsky/shortener/internal/handlers.handleShorten
         0     0% 23.93%  -768.26kB 15.60%  github.com/AlexeyKurlevsky/shortener/internal/logger.Initialize
         0     0% 23.93%   655.29kB 13.30%  github.com/AlexeyKurlevsky/shortener/internal/middleware.(*compressWriter).Close
         0     0% 23.93%   512.20kB 10.40%  github.com/AlexeyKurlevsky/shortener/internal/middleware.AuthMiddleware.func1.1
         0     0% 23.93%  1167.50kB 23.70%  github.com/AlexeyKurlevsky/shortener/internal/middleware.GzipMiddleware.func1
         0     0% 23.93%  1167.50kB 23.70%  github.com/AlexeyKurlevsky/shortener/internal/middleware.RequestLogger.func1
         0     0% 23.93%   512.20kB 10.40%  github.com/AlexeyKurlevsky/shortener/internal/storage.(*PostgresStorage).FindIDByURL
         0     0% 23.93%  1167.50kB 23.70%  github.com/go-chi/chi/v5.(*Mux).ServeHTTP
         0     0% 23.93%   512.20kB 10.40%  github.com/go-chi/chi/v5.(*Mux).routeHTTP
         0     0% 23.93%  1167.50kB 23.70%  github.com/go-chi/chi/v5/middleware.Recoverer.func1
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5.(*Conn).Query
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5.(*Conn).QueryRow (inline)
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5.(*ExtendedQueryBuilder).Build
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5.(*ExtendedQueryBuilder).appendParam
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5.(*ExtendedQueryBuilder).encodeExtendedParamValue
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5/pgtype.(*Map).Encode
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5/pgtype.(*Map).PlanEncode (inline)
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5/pgxpool.(*Conn).QueryRow
         0     0% 23.93%   512.20kB 10.40%  github.com/jackc/pgx/v5/pgxpool.(*Pool).QueryRow
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.(*Logger).WithOptions
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.Config.Build
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.Config.buildOptions.func1
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.New
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.WrapCore.func1
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap.optionFunc.apply
         0     0% 23.93%  -768.26kB 15.60%  go.uber.org/zap/zapcore.NewSamplerWithOptions
         0     0% 23.93%  -810.13kB 16.45%  main.main
         0     0% 23.93%  1167.50kB 23.70%  net/http.(*conn).serve
         0     0% 23.93%  1167.50kB 23.70%  net/http.HandlerFunc.ServeHTTP
         0     0% 23.93%  1167.50kB 23.70%  net/http.serverHandler.ServeHTTP
         0     0% 23.93%  -512.01kB 10.40%  runtime.(*scavengerState).sleep
         0     0% 23.93%  -512.01kB 10.40%  runtime.(*timer).maybeAdd
         0     0% 23.93%  -512.01kB 10.40%  runtime.(*timer).modify
         0     0% 23.93%  -512.01kB 10.40%  runtime.(*timer).reset (inline)
         0     0% 23.93%  -512.01kB 10.40%  runtime.(*timers).addHeap
         0     0% 23.93%  -512.01kB 10.40%  runtime.bgscavenge
         0     0% 23.93%  -512.01kB 10.40%  runtime.growslice
         0     0% 23.93%  -810.13kB 16.45%  runtime.main
         0     0% 23.93%  -512.01kB 10.40%  runtime.mallocgc
```