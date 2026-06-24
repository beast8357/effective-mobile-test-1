# Subscriptions API

REST-сервис для агрегации данных об онлайн-подписках пользователей
(тестовое задание Effective Mobile).

## Возможности

- CRUDL для записей о подписках (`/subscriptions`).
- Подсчёт суммарной стоимости подписок за период с фильтрами по
  `user_id` и `service_name` (`/subscriptions/total`).
- Миграции PostgreSQL применяются автоматически на старте сервиса.
- Структурированные логи (`log/slog`, JSON).
- Конфигурация через `config.yaml` + override переменными окружения.
- Swagger UI на `/swagger`, спецификация — `/swagger.yaml`.

## Запуск

```bash
docker compose up --build
```

После старта:

- API:        http://localhost:8080
- Swagger UI: http://localhost:8080/swagger
- Спека:      http://localhost:8080/swagger.yaml
- Liveness:   http://localhost:8080/healthz

Параметры можно переопределить через переменные окружения (см. `.env.example`).
Например, `HTTP_PORT=9000 docker compose up --build`.

## Локальный запуск (без docker)

Нужен Go 1.22+ и поднятый PostgreSQL.

```bash
go mod tidy
DB_HOST=localhost DB_PASSWORD=postgres go run .
```

## Эндпоинты

| Метод  | Путь                       | Описание                                       |
|--------|----------------------------|------------------------------------------------|
| POST   | `/subscriptions`           | Создать подписку                               |
| GET    | `/subscriptions`           | Список с фильтрами `user_id`, `service_name`   |
| GET    | `/subscriptions/{id}`      | Получить одну подписку                         |
| PUT    | `/subscriptions/{id}`      | Заменить подписку                              |
| DELETE | `/subscriptions/{id}`      | Удалить подписку                               |
| GET    | `/subscriptions/total`     | Сумма за период `from..to` (формат `MM-YYYY`)  |
| GET    | `/healthz`                 | Liveness                                       |
| GET    | `/swagger`                 | Swagger UI                                     |

### Пример создания

```bash
curl -X POST http://localhost:8080/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{
    "service_name": "Yandex Plus",
    "price": 400,
    "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
    "start_date": "07-2025"
  }'
```

### Пример агрегации

```bash
curl 'http://localhost:8080/subscriptions/total?from=01-2025&to=12-2025&user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba'
```

Логика подсчёта: для каждой подписки, пересекающейся с окном `[from, to]`,
считается число активных месяцев внутри окна (`max(start, from) .. min(end ?? to, to)`,
включительно) и умножается на `price`.

## Структура

```
.
├── main.go                       # точка входа, embed миграций и swagger.yaml
├── config.yaml                   # дефолтная конфигурация
├── docker-compose.yml            # postgres + api
├── Dockerfile                    # multi-stage build (distroless)
├── docs/swagger.yaml             # OpenAPI 3.0 спецификация
├── migrations/
│   ├── 000001_init.up.sql
│   └── 000001_init.down.sql
└── internal/
    ├── config/                   # загрузка yaml + env (cleanenv)
    ├── logger/                   # slog wrapper
    ├── middleware/               # logger + recover
    ├── model/                    # доменная модель + MonthYear (MM-YYYY)
    ├── repository/               # pgx + миграционный раннер
    └── handler/                  # HTTP, валидация, роутинг
```
# effective-mobile-test-1
