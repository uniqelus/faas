# MVP: обзор

Документ-«входная точка» в планирование MVP платформы. Здесь — только скоуп и
навигация. Все обоснования вынесены в Architecture Decision Records в
`docs/decisions/`. Конкретные задачи живут в GitHub Issues и группируются по
вехам M0–M4 (см. раздел «Вехи» ниже).

## Что такое MVP

Локально разворачиваемая FaaS-платформа на Go и Kubernetes, в которой один
разработчик может пройти полный жизненный цикл функции и подтвердить работу
платформы интеграционными и нагрузочными тестами.

**В скоупе:**

- Регистрация функции (`POST /v1/functions`).
- Деплой как `Deployment` + `Service` в namespace `faas-fns`.
- Инвокация через HTTP gateway: `POST /fn/{name}`.
- Inspect статуса, чтение логов (с follow).
- Обновление и удаление функции.
- Структурные логи, метрики Prometheus, трейсинг OpenTelemetry → Jaeger.
- Воспроизводимый стенд: `terraform apply` поднимает кластер + Helm-релиз.
- Полный набор интеграционных e2e-тестов и нагрузочных сценариев k6.

**Вне скоупа (отложено в Phase 2+):**

- Сборка функций из исходников (Buildpacks / kaniko).
- Scale-to-zero, concurrency-based autoscaling.
- Авторизация, мульти-тенантность, RBAC поверх функций.
- Версионирование с traffic split / canary.
- Async-инвокации, event sources.
- Не-Go рантаймы (WASM, Python, Node).
- CRD + operator (см. ADR 0006 — отложено сознательно).
- Backup/restore Postgres, production-grade Jaeger storage.

## Принятые решения

| #    | Решение                                  | Документ                                                   |
| ---- | ---------------------------------------- | ---------------------------------------------------------- |
| 0001 | Локальный кластер — `k3d` (K3s в Docker) | [0001](decisions/0001-local-cluster-k3d.md)                |
| 0002 | Control API — gRPC + grpc-gateway        | [0002](decisions/0002-control-api-grpc-gateway.md)         |
| 0003 | Функции — pre-built OCI-образы           | [0003](decisions/0003-pre-built-function-images.md)        |
| 0004 | Observability в скоупе MVP               | [0004](decisions/0004-observability-in-mvp.md)             |
| 0005 | Один umbrella Helm-чарт `faas-platform`  | [0005](decisions/0005-helm-umbrella-chart.md)              |
| 0006 | Plain API + Postgres, без CRD/operator   | [0006](decisions/0006-no-crd-plain-api-with-postgres.md)   |

## Definition of Done MVP

В конце MVP без ручных шагов должен отрабатывать следующий скрипт:

```bash
make infra-up                          # k3d + helm-релиз платформы
make images && make images-load        # control-plane + gateway в k3d
make example-build EXAMPLE=hello-go    # билд функции-эталона
k3d image import faas/hello-go:dev -c faas

curl -X POST http://localhost:8080/v1/functions \
  -H 'content-type: application/json' \
  -d '{"name":"hello","image":"faas/hello-go:dev","replicas":2}'

curl http://localhost:8080/v1/functions/hello              # status=ready
curl -X POST http://localhost:8080/fn/hello -d 'world'     # echo
curl -N "http://localhost:8080/v1/functions/hello/logs?follow=true"

# В Grafana: faas_invoke_duration_seconds p95, faas_reconcile_duration_seconds
# В Jaeger : trace gateway → pod, trace control-plane → reconciler

curl -X DELETE http://localhost:8080/v1/functions/hello
make test-e2e                          # все сценарии зелёные
make load-baseline                     # smoke + steady, отчёт в tests/load/
make infra-down
```

Если хоть один шаг не работает — MVP не закрыт.

## Структура репозитория после MVP

```
api/proto/faas/v1/*.proto              # контракт API
build/docker/*.Dockerfile              # сборка образов сервисов
cmd/api-gateway/                       # бинарь gateway
cmd/control-plane/                     # бинарь control-plane
configs/*.example.yaml                 # примеры конфигов
deployments/
  helm/faas-platform/                  # umbrella-чарт
  migrations/*.sql                     # миграции Postgres
  terraform/                           # cluster + helm_release
docs/
  mvp-overview.md                      # этот файл
  decisions/                           # ADR
examples/hello-go/                     # эталонная функция
internal/
  api-gateway/                         # invoke, resolver
  control-plane/                       # api/, reconciler/, storage/, k8s/
  domain/                              # Function, валидация
pkg/
  api/faas/v1/                         # сгенерированный gRPC + gateway
  config/, errors/, log/               # уже есть
  grpc/, http/, grpcgateway/           # серверные компоненты
  observability/                       # OTel + Prometheus + zap-hook
tests/
  e2e/                                 # интеграционные (build tag e2e)
  load/                                # k6
  contract/                            # buf breaking + smoke
```

## Вехи

| ID     | Цель                                                                   | Demo-критерий                                                                |
| ------ | ---------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| **M0** | Фундамент: proto, серверные компоненты, observability skeleton, бинари | `make binaries` собирает оба бинаря, оба отдают `/healthz` и `/metrics`      |
| **M1** | Инфра как код: k3d + Helm + Postgres + observability-стек              | `terraform apply` с нуля даёт работающую пустую платформу + Prometheus/Jaeger |
| **M2** | Полный lifecycle функции с метриками и трейсами                        | Весь demo-скрипт выше работает руками (без `test-e2e`/`load-baseline`)       |
| **M3** | Интеграционные тесты + CI                                              | `make test-e2e` зелёный, GitHub Actions PR-чек зелёный                       |
| **M4** | Нагрузочные тесты + baseline + дашборды                                | `make load-baseline` сохраняет отчёт, `docs/load-baseline.md` существует     |

Задачи внутри каждой вехи — в GitHub Issues с лейблами `milestone:M0` … `milestone:M4`.

## Архитектура (краткая выжимка)

**Компоненты:**

1. `control-plane` (`cmd/control-plane`) — gRPC + HTTP/JSON через grpc-gateway,
   Postgres под метаданными, in-process реконсайлер на `workqueue` через
   `client-go` server-side apply.
2. `api-gateway` (`cmd/api-gateway`) — HTTP-фронт для invoke. Резолвит имя
   функции в K8s Service, проксирует через `httputil.ReverseProxy`. Без стейта.
3. `postgres` — Bitnami subchart в namespace `faas-system`.
4. Функции — обычные `Deployment` + `ClusterIP Service` в `faas-fns` с лейблами
   `faas.io/function=<name>`, `faas.io/version=<v>`.

**Request flow (invoke):**

```
client → ingress → svc/api-gateway → gateway lookup name
       → svc/<fn>.faas-fns → pod → response
```

**Deploy flow:**

```
client → POST /v1/functions → control-plane:
  1. validate
  2. INSERT INTO functions ... RETURNING version
  3. enqueue reconcile(name)
reconciler (async):
  4. read desired from DB
  5. server-side apply Deployment
  6. server-side apply Service
  7. wait readyReplicas ≥ 1 (timeout)
  8. UPDATE functions SET status=...
```

**Failure modes:** см. ADR 0006 (sec. «Последствия»).

## Риски и неизвестные

1. `go.mod` использует Go 1.26.2 — проверить совместимость с `client-go`,
   `pgx`, `controller-runtime` (если затащим типы) — в самой первой задаче M0.
2. `k3d` в CI требует Docker-in-Docker. GitHub-hosted runners это умеют; для
   self-hosted нужно проверить.
3. Observability-стек (Prometheus + Grafana + Jaeger + OTel Collector) даёт
   ~300–500 MB RAM в кластере. Профиль `lite` через `values.yaml` для слабых
   машин — обязателен.
4. Единственный writer в K8s — реконсайлер. Никаких прямых `Apply` из
   handler-ов API.
