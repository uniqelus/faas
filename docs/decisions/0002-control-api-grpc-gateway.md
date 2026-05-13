# 0002. Control API — gRPC + grpc-gateway

- Статус: Accepted
- Дата: 2026-05-13

## Контекст

Платформе нужен управляющий API для control-plane:

- CRUD над функциями (`Create`, `Get`, `List`, `Update`, `Delete`).
- Чтение логов с поддержкой стрима (`GetLogs`).

Существующий скелет `cmd/api-gateway` уже опирается на `pkg/grpc` — значит,
инфраструктура для gRPC в проекте уже выбрана. При этом для demo-сценария и
ручного тестирования удобнее HTTP/JSON: `curl`, Postman, любой браузер.

Цель: один контракт, два транспорта, без дублирования кода и риска расхождения.

## Решение

API описан в `.proto`-файлах в `api/proto/faas/v1/`. Из них генерируется:

- gRPC-сервер и клиент (`protoc-gen-go`, `protoc-gen-go-grpc`).
- HTTP/JSON-фасад через **grpc-gateway** (`protoc-gen-grpc-gateway`).
- OpenAPI v2-спека (`protoc-gen-openapiv2`) — побочный артефакт, можно
  выставить через Swagger UI.

Кодген выполняется через **`buf`** (`buf.build`):

- `buf.yaml` / `buf.gen.yaml` в `api/proto/`.
- Цели `make proto-gen`, `make proto-lint`, `make proto-breaking`.

`control-plane` слушает два порта:

- gRPC — `:9000` (внутри кластера, используют sidecar-ы / интеграционные тесты).
- HTTP/JSON — `:8080` (используют пользователи через ingress).

Серверный стриминг (`GetLogs`) маппится в HTTP `Transfer-Encoding: chunked`
через `grpc-gateway/runtime`.

## Рассмотренные альтернативы

| Вариант | Плюсы | Минусы |
| --- | --- | --- |
| **Pure HTTP/JSON** (`chi` + struct tags) | Минимум зависимостей, проще для разработчиков без proto-опыта | Контракт неявный, нет типизированных клиентов, нет breaking-checks; стриминг логов — вручную через SSE/chunked |
| **Pure gRPC** | Типизировано, эффективно, генерится клиент | Плохо демонстрируется (нужен `grpcurl`), нельзя по-простому показать в demo |
| **HTTP-first + OpenAPI как source of truth** | Идиоматично для REST-команд | Нужен отдельный кодгенератор клиентов, спека и реализация легко расходятся |
| **gRPC + grpc-gateway** (выбран) | Один source of truth (`.proto`), оба транспорта, бесплатный OpenAPI, breaking-checks в CI через `buf` | Толще toolchain (нужен `buf`), больше boilerplate, ramp-up на новой технологии |

## Последствия

**Положительные:**

- `.proto` — единственный источник правды о контракте. Не бывает «в коде так,
  а в доке этак».
- `buf breaking` в CI ловит обратную несовместимость до merge.
- Сгенерированный openapi v2 можно выкатить через Swagger UI в control-plane —
  бесплатная интерактивная документация.
- Типизированные gRPC-клиенты для интеграционных тестов и будущих SDK.

**Отрицательные:**

- В toolchain появляется `buf` и набор `protoc-gen-*` плагинов.
- HTTP-маршруты описываются через `google.api.http` аннотации в `.proto` —
  ещё одна нотация для команды.
- Стриминговые ручки требуют отдельной аккуратности: маппинг в HTTP
  ограничен chunked-ответом, нужно правильно ставить таймауты и
  flush-стратегии.
- В `pkg/grpcgateway/` появляется свой server-component, который склеивает
  gRPC и HTTP через bufconn или localhost socket — это дополнительный код.

**Точечно:**

- В `pkg/api/faas/v1/` — сгенерированные файлы (коммитим в git, не генерим
  каждый раз в CI).
- ServerStream `GetLogs` в grpc-gateway маппится как HTTP с `chunked` —
  `grpc-gateway/runtime` это умеет, но нужно проверить настройки sampling
  в OTel, чтобы не схлопывало long-lived span.

## Триггеры пересмотра

- Если решим, что API будет публичным и RESTful-идиоматичным (с PATCH, ETag,
  HATEOAS-нюансами), и эти ограничения станут давить — мигрировать на
  HTTP-first.
- Если стриминг логов будет требовать продвинутого UX (resume, seek,
  multiplexing) — рассмотреть отдельный WebSocket-эндпоинт вне grpc-gateway.

## Связанные решения

- ADR 0006 — control-plane как plain Go-сервис; gRPC API живёт внутри него,
  без CRD/operator.
