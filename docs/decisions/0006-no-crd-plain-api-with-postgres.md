# 0006. Plain API + Postgres, без CRD/operator

- Статус: Accepted
- Дата: 2026-05-13

## Контекст

«Естественный» для Kubernetes-нативного FaaS дизайн — это CRD `Function` плюс
оператор на `controller-runtime`. Так делают Knative, OpenFaaS (новые
версии), Fission, kpack. У такого подхода есть очевидные плюсы:

- Kubernetes сам является базой состояния — не нужен отдельный Postgres.
- `kubectl get functions` / `kubectl describe function foo` — бесплатный DX.
- Реконсайлинг-паттерн встроен (`controller-runtime`): workqueue с backoff,
  leader election, watch-кэш, finalizers, owner references.
- Status subresource с `status.conditions` — стандартизованная схема.
- GitOps-friendly: Argo CD / Flux синхронизируют CR из git без отдельной API.

При этом правило `10-architecture.mdc` репозитория явно говорит:

> Prefer CRD only if it clearly simplifies lifecycle management; otherwise
> start with plain API + manifests.

Это сознательная установка на максимально короткий путь до demo. Вопрос:
делать ли исключение из правила для нашего MVP?

## Решение

CRD/operator в MVP **не вводим**. Архитектура control-plane:

- `cmd/control-plane` — обычный Go-сервис.
- Метаданные функций — в Postgres (`functions` таблица: `name`, `image`,
  `env`, `resources`, `replicas`, `version`, `status`, `last_error`,
  `created_at`, `updated_at`).
- Применение в кластер — реконсайлер-горутина с `workqueue.RateLimitingQueue`
  из `client-go`, который делает server-side apply для `Deployment` и
  `Service`.
- БД — источник правды о намерении; K8s — производное состояние.
- Реконсайлер — единственный writer в K8s API (исключает race с
  handler-ами).

Источники событий для реконсайлера:

1. Bootstrap — list всех функций из БД на старте, enqueue.
2. API handler-ы — enqueue после успешной транзакции в БД.
3. Periodic resync — каждые ~30s (защита от пропусков и от рассинхрона с
   K8s).

Repository layer — это интерфейс. В Phase 2, если решим переезжать на CRD,
реализация репозитория меняется на «обёртку над K8s API» без перелопачивания
бизнес-логики.

## Рассмотренные альтернативы

| Вариант | Плюсы | Минусы |
| --- | --- | --- |
| **CRD + kubebuilder operator** | Меньше кода (генерация), `kubectl get functions` бесплатно, GitOps-friendly, индустриальный стандарт | Ramp-up controller-runtime, coupling всего стейта к K8s API, etcd как «прод-БД приложения» с её ограничениями (1.5MB на объект, watch-лимиты, write-шумы влияют на сам K8s) |
| **Postgres + синхронный POST → apply внутри handler-а** | Просто, без отдельного воркера | Любая транзиентная ошибка K8s = расхождение БД и кластера; нет retry без своей реализации; race на параллельные апдейты |
| **Postgres + реконсайлер** (выбран) | Чёткая граница «намерение / факт», единственный writer в K8s, естественный retry/backoff | Нужно своё: workqueue, leader election (если потребуется), resync; +Postgres в Helm-чарт |
| **In-memory + только K8s** (без БД, но без CRD) | Самый простой код | Состояние теряется при рестарте control-plane; нет истории |

## Последствия

**Положительные:**

- Понятный mental model: «БД хранит, что просили; K8s показывает, как
  получилось».
- Бэкап — это `pg_dump`, восстановление — это `psql`. Знакомые инструменты.
- Реконсайлер легко тестируется: фейковый `kubernetes.Interface` из
  `client-go/kubernetes/fake` плюс testcontainers-Postgres дают полный
  unit/integration набор без поднятия кластера.
- В будущем легко добавить аудит, историю деплоев, мульти-кластер — всё в БД.

**Отрицательные:**

- Больше кода: свой workqueue-цикл, своё ожидание readiness, своя обработка
  resync. Контролле-runtime всё это даёт из коробки.
- Postgres — обязательная зависимость, со своими операционными нюансами
  (миграции, бэкапы, версии PG).
- `kubectl get function foo` **не работает** — функции невидимы для
  kubectl напрямую (видны только их `Deployment`/`Service` с лейблами).
- GitOps-сценарий «закоммитил CR — кластер подтянул» отсутствует. Чтобы
  его получить, нужен наш собственный API или путь поверх него.

**Failure modes (важно зафиксировать):**

| Сбой | Поведение |
| --- | --- |
| Postgres недоступен на старте | control-plane не стартует (fail fast). Gateway — продолжает работать в режиме «инвоки идут, но управление недоступно». Это возможно только если gateway не зависит от БД (резолвит имя функции через K8s DNS / Endpoints API) — так и делаем |
| K8s API недоступен | Реконсайлер ретраит с backoff, `last_error` пишется в БД, статус функции — `degraded` |
| `ImagePullBackOff` | Реконсайлер ловит это в `Deployment.status`, пишет понятное сообщение в `last_error`; `GET /v1/functions/{name}` показывает его |
| Функция в `CrashLoopBackOff` | То же: `status=unhealthy`, видно в inspect |
| Сам control-plane упал между записью в БД и enqueue | Periodic resync через 30s подберёт несогласованность |

## Триггеры пересмотра

- Если пользователи начнут просить GitOps-сценарий (Argo CD / Flux) и
  `kubectl`-нативный UX — мигрируем в CRD-вариант. Миграция стейта: написать
  одноразовую джобу, которая прочитает `functions` из БД и создаст CR.
- Если число функций превысит несколько тысяч и SQL-запросы по статусам
  станут узким местом — это аргумент **за** оставаться на БД, не за CRD
  (etcd ещё хуже на таких объёмах).
- Если кто-то из команды очень хорошо умеет в `kubebuilder` и готов делать
  Phase 2 сам — переход на CRD становится дешевле, его можно
  рассматривать раньше.

## Связанные решения

- ADR 0002 — control-plane экспонирует API через gRPC + grpc-gateway; это
  единственный путь управления (kubectl напрямую не работает).
- ADR 0005 — Postgres подключён как subchart umbrella-чарта.
- ADR 0004 — реконсайлер инструментирован метрикой
  `faas_reconcile_duration_seconds` и спанами; status в БД дублируется
  gauge-метрикой `faas_function_status`.
