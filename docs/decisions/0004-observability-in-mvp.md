# 0004. Observability в скоупе MVP

- Статус: Accepted
- Дата: 2026-05-13

## Контекст

Соблазн отложить наблюдаемость «на потом» велик и стандартен. Аргументы «за
отложить»: меньше зависимостей, быстрее до первого работающего invoke.

Аргументы против:

1. **Нагрузочные тесты** (один из критериев DoD MVP) без метрик дают только
   результат `k6` — нельзя соотнести с тем, где именно деградирует
   платформа.
2. **Дебаг e2e-тестов** без трейсов превращается в `kubectl logs` по трём
   подам с попыткой склеить запросы по таймстампам.
3. **Холодный старт и латентность gateway** — типичная проблема FaaS;
   измерять её придётся в любом случае, лучше с самого начала.
4. **Добавлять observability к написанному коду** заметно дороже, чем писать
   код сразу с инструментированием.

## Решение

Метрики и трейсинг входят в скоуп MVP с самого начала.

**Стек:**

- Метрики: `prometheus/client_golang` → Prometheus (lite, без kube-prometheus-
  stack) → Grafana с preload-дашбордом.
- Трейсинг: OpenTelemetry SDK → OTel Collector (OTLP gRPC receiver) → Jaeger
  all-in-one (memory storage).
- Логи: zap (уже есть) + hook, который дописывает `trace_id`/`span_id` из
  активного span в каждую запись.

**Бизнес-метрики MVP:**

| Сервис | Метрика | Тип | Лейблы |
| --- | --- | --- | --- |
| gateway | `faas_invoke_duration_seconds` | histogram | `fn`, `status` |
| gateway | `faas_invoke_total` | counter | `fn`, `status` |
| gateway | `faas_invoke_in_flight` | gauge | `fn` |
| gateway | `faas_lookup_duration_seconds` | histogram | — |
| control-plane | `faas_reconcile_duration_seconds` | histogram | `result` |
| control-plane | `faas_reconcile_queue_depth` | gauge | — |
| control-plane | `faas_function_status` | gauge | `name`, `status` |
| control-plane | `faas_api_request_duration_seconds` | histogram | `method`, `path`, `code` |

**Спаны (минимум):**

```
HTTP POST /fn/{name}                       (gateway)
├── faas.lookup_service                    (gateway)
└── HTTP POST → pod                        (gateway, otelhttp)
    └── (опц., если функция инструментирована — её спан)

POST /v1/functions                         (control-plane)
├── db.insert function                     (otelpgx)
└── enqueue reconcile

faas.reconcile <name>                      (control-plane, async)
├── k8s.apply deployment
└── k8s.apply service
```

**Расположение в коде:**

- `pkg/observability/` — общая инициализация (tracer provider, meter provider,
  Prom registry, zap hook).
- В Helm-чарте — отдельные subcharts под `condition: observability.metrics.enabled`
  и `condition: observability.tracing.enabled`, чтобы можно было выключить
  для слабых машин / CI.

## Рассмотренные альтернативы

| Вариант | Почему не |
| --- | --- |
| **Отложить полностью (M4-only)** | Нагрузочные тесты в M4 без observability бесполезны для диагностики; ретрофит дороже |
| **Только метрики, без трейсов** | Метрики дают «что», трейсы дают «где»; для распределённой системы (gateway → fn) без трейсов разбираться долго |
| **OpenTelemetry для метрик тоже** (вместо Prom client) | Идеологично, но Prom client_golang стабильнее и проще для bridge в Prometheus scrape |
| **`kube-prometheus-stack`** | Полный стек тянет Alertmanager, node-exporter, kube-state-metrics, prometheus-operator. Для MVP избыточно. Берём минимальный набор. |
| **Tempo вместо Jaeger** | Jaeger проще ставится all-in-one и привычнее; Tempo выгоден на больших объёмах, что не относится к MVP |

## Последствия

**Положительные:**

- Нагрузочные тесты сразу дают разложение латентности по этапам.
- Дебаг ошибок упрощается: один `trace_id` ведёт через все компоненты.
- Связка логи↔трейсы через `trace_id` в каждом zap-сообщении — это «бесплатно»
  при правильной инициализации.
- Все handler-ы и реконсайлер инструментируются с момента написания —
  никаких «на следующей итерации добавим».

**Отрицательные:**

- В кластере появляется ~3–4 дополнительных пода (Prometheus, Grafana,
  Jaeger, OTel Collector). Это ~300–500 MB RAM и заметный CPU.
- Нужен профиль `values.lite.yaml` (без observability) — обязательно для
  слабых ноутов и для GitHub Actions runner-ов, иначе кластер не помещается.
- `helm install` платформы становится дольше (~3–5 минут вместо ~1).
- В CI-сборке `e2e` тоже придётся учитывать профиль (рекомендуется `lite`,
  чтобы e2e быстрее).

**Точечно:**

- В каждом сервисе появляется отдельный admin-listener (`/metrics`,
  `/healthz`) — отдельно от бизнес-портов. Это важно для security: бизнес-
  порт может быть закрыт от мира, admin-порт — только для Prometheus.
- В zap-логах появляется поле `service` (имя бинаря) и автоматически
  `trace_id`/`span_id`. Это меняет схему логов — её нужно зафиксировать в
  ADR логирования (отдельно, если потребуется).

## Триггеры пересмотра

- Если CI runners не справляются с весом observability-стека даже на
  `lite` — рассматривать отдельную «голову» CI, где e2e гоняются без
  метрик/трейсов, и dedicated job, где гоняются только observability-проверки.
- Если решим в Phase 2 идти в прод — менять Jaeger memory storage на
  ElasticSearch/Cassandra и Prometheus на отдельный stateful-стек.

## Связанные решения

- ADR 0005 — observability-стек подключён как subcharts umbrella-чарта с
  feature-флагами.
- ADR 0002 — у gRPC и HTTP свои интерсепторы для трейсов и метрик
  (`otelgrpc`, `otelhttp`).
