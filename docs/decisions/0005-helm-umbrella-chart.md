# 0005. Один umbrella Helm-чарт

- Статус: Accepted
- Дата: 2026-05-13

## Контекст

Платформа состоит из нескольких компонентов:

- `control-plane` (наш бинарь)
- `api-gateway` (наш бинарь)
- Postgres (метаданные функций)
- Prometheus + Grafana (метрики)
- Jaeger + OTel Collector (трейсы)

Их нужно как-то упаковывать для установки в кластер. Возможные стратегии:

1. Один **umbrella-чарт**, который ставит всё разом.
2. **Несколько отдельных чартов** — по одному на компонент или группу.
3. **Без Helm** — kustomize или plain манифесты.
4. **Только Terraform** — без Helm, через `kubernetes_*` ресурсы.

Контекст принятия решения:

- Один разработчик в MVP, и demo-сценарий — «`terraform apply` и работает».
- Стейтфул-зависимости (Postgres) и стейтлесс наши сервисы имеют разные
  релизные циклы — теоретически.
- На demo это не важно, но в проде — важно.

## Решение

Один umbrella Helm-чарт `deployments/helm/faas-platform/`. Сторонние
компоненты (Postgres, Prometheus, Grafana, Jaeger, OTel Collector)
подключаются как subcharts через `dependencies` в `Chart.yaml`. Тяжёлые
зависимости прячутся за `condition:`, что даёт профили:

- **Full** (по умолчанию) — всё включено, для локального дев и demo.
- **Lite** — `observability.metrics.enabled=false`, `observability.tracing.enabled=false`,
  только Postgres + наши сервисы. Для слабых машин и CI.
- **External-pg** — `postgresql.enabled=false`, ожидается внешний Postgres,
  данные подключения через `values.yaml`. Заготовка под прод.

Terraform ставит umbrella через один `helm_release`. Корневой `terraform
apply` поднимает кластер и тут же релизит платформу.

Postgres PVC помечается `helm.sh/resource-policy: keep`, чтобы случайный
`helm uninstall` не уничтожил данные. Снос данных — отдельной командой /
ручкой.

## Рассмотренные альтернативы

| Вариант | Плюсы | Минусы |
| --- | --- | --- |
| **Отдельные чарты, отдельные релизы** | Независимые релизные циклы, понятная ownership-граница | 3+ команд установки, синхронизация версий вручную, дублирование values |
| **Без Helm — kustomize + plain манифесты** | Меньше абстракций | Нет управления зависимостями, нет параметризации значениями — придётся писать своё |
| **Только Terraform** (`kubernetes_*` ресурсы) | Один инструмент в скоупе | Дублирует функции Helm (template, dependencies), плохо работает с upstream-чартами |
| **Umbrella + subcharts** (выбран) | Один `helm_release`, единые values, условные зависимости | При сильном расхождении релизных циклов компонентов umbrella становится тесным |

## Последствия

**Положительные:**

- Один `helm_release` в Terraform — короткий root-модуль.
- Один источник параметров (`values.yaml`) — `db password` упоминается в одном
  месте, его подтягивают и Postgres-subchart, и control-plane.
- Профили (`full`/`lite`/`external-pg`) дают линию миграции к проду без
  переписывания структуры.
- `helm template` рендерит весь стенд для ревью.

**Отрицательные:**

- При обновлении одной из subcharts-зависимостей нужно бамать версию umbrella
  и релизить всё разом. В MVP это не критично, в проде — раздражает.
- Postgres-релиз привязан к жизни umbrella. Митигация — `keep`-policy на PVC
  и плановое разнесение в Phase 2.
- Размер папки `templates/` растёт быстро — нужно держать структуру:
  `templates/control-plane/`, `templates/api-gateway/`, `templates/dashboards/`.

**Точечно:**

- `values.yaml` — единая поверхность настройки. Изменения в `values.schema.json`
  валидируются Helm-ом, что снижает риск опечаток.
- В CI добавляются проверки: `helm lint`, `helm template | kubeval` (или
  `kubeconform`).

## Триггеры пересмотра

- Если придётся часто релизить только gateway или только control-plane без
  трогания остального — разнести их в отдельные чарты с общим
  `values-shared.yaml`.
- Если в Phase 2 Postgres переедет на managed-сервис (или будет жить отдельно
  как stateful-релиз) — `postgresql` уйдёт из umbrella, останется как
  отдельный релиз / внешняя БД.

## Связанные решения

- ADR 0001 — `k3d` использует встроенный Traefik, и umbrella использует его
  как ingress; отдельный nginx-ingress-controller в чарт **не** включаем.
- ADR 0004 — observability-стек встроен как subcharts с feature-флагами.
- ADR 0006 — Postgres-зависимость в umbrella существует из-за решения хранить
  метаданные в БД, а не в etcd через CRD.
