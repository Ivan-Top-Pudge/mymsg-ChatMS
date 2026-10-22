# mymsg-ChatMS

[![CI Pipeline](https://github.com/Ivan-Top-Pudge/mymsg-ChatMS/actions/workflows/ci.yml/badge.svg)](https://github.com/Ivan-Top-Pudge/mymsg-ChatMS/actions/workflows/ci.yml)

Микросервис чатов и сообщений для проекта **mymsg**, написанный на Go. Предоставляет gRPC API для создания чатов, отправки и удаления сообщений, а также получения истории переписки. Данные хранятся в PostgreSQL, проверки участия в чате при отправке сообщений кэшируются в Redis, а существование пользователей проверяется через отдельный SSO-сервис.

## Возможности

- Создание чатов с проверкой участников через SSO.
- Удаление чатов с каскадным удалением сообщений и связей участников в PostgreSQL.
- Отправка текстовых сообщений участниками чата.
- Удаление сообщения его автором.
- Получение истории сообщений с пагинацией `limit` / `offset`, от новых к старым.
- Аутентификация gRPC-запросов по JWT через unary interceptor.
- Кэширование результатов проверки участия в Redis с настраиваемым TTL и обращением к PostgreSQL при промахе или ошибке кэша.
- SQL-миграции, модульные и интеграционные тесты, CI в GitHub Actions.

## Архитектура

```mermaid
flowchart TD
    Client["gRPC-клиент"] --> API["ChatMS: JWT interceptor и обработчики"]
    API --> Service["Бизнес-логика чатов"]
    Service --> DB["PostgreSQL: чаты, участники, сообщения"]
    Service --> Cache["Redis: участие в чате"]
    Service --> SSO["SSO: проверка пользователей"]
```

Обработчики проверяют параметры запроса и извлекают пользователя из JWT. Сервисный слой выполняет проверки доступа и обращается к хранилищу, кэшу и SSO через интерфейсы.

## Технологии

| Компонент | Использование |
| --- | --- |
| Go 1.26.0 | Версия, указанная в `go.mod` |
| gRPC / Protocol Buffers | API и межсервисное взаимодействие |
| PostgreSQL / pgx v5 | Основное хранилище и пул соединений |
| Redis / go-redis v9 | Кэш проверок участия в чате |
| golang-jwt/jwt v5 | Проверка JWT |
| cleanenv | Чтение YAML-конфигурации |
| golang-migrate | Применение и откат SQL-миграций |
| testify / GoMock | Модульные тесты и моки |
| Testcontainers | Интеграционные тесты с PostgreSQL 17 |
| Task | Команды запуска и миграций |

Контракты подключены как Go-модули:

- [mymsg-protos](https://github.com/Ivan-Top-Pudge/mymsg-protos/tree/v0.0.1) — API чатов, версия `v0.0.1`.
- [protos](https://github.com/Ivan-Top-Pudge/protos/tree/v0.0.3) — API SSO, версия `v0.0.3`.

## Локальный запуск

### 1. Подготовка

Понадобятся Go 1.26.0 или совместимая более новая версия, PostgreSQL, Redis и запущенный SSO-сервис с методом `IsUserExists`. Для примеров ниже нужен Docker; Task и grpcurl используются по желанию.

```bash
git clone https://github.com/Ivan-Top-Pudge/mymsg-ChatMS.git
cd mymsg-ChatMS
go mod download
```

### 2. PostgreSQL и Redis

Пример запуска зависимостей для локальной разработки:

```bash
docker run -d --name mymsg-chat-postgres \
  -e POSTGRES_USER=chat \
  -e POSTGRES_PASSWORD=chat \
  -e POSTGRES_DB=chat \
  -p 127.0.0.1:5432:5432 \
  -v mymsg-chat-postgres-data:/var/lib/postgresql/data \
  postgres:17

docker run -d --name mymsg-chat-redis \
  -p 127.0.0.1:6379:6379 \
  redis:7
```

SSO разворачивается отдельно. Он должен обслуживать контракт `github.com/Ivan-Top-Pudge/protos/gen/go/sso`; адрес указывается в `sso_address`.

### 3. Конфигурация

Создайте каталог `config` и файл `config/local.yaml`: готовый файл конфигурации в репозиторий не включён.

```bash
mkdir -p config
```

```yaml
env: local
postgres_dsn: "postgres://chat:chat@localhost:5432/chat?sslmode=disable"
sso_address: "localhost:44044"
jwt_secret: "replace-with-your-sso-signing-secret"
redis_url: "redis://localhost:6379/0"
redis_ttl: 5m

grpc:
  port: 44045
  timeout: 5s
```

Порты, учётные данные и TTL в примере — параметры локальной настройки. Укажите фактический адрес SSO и согласуйте `jwt_secret` с ключом подписи токенов.

| Параметр | Назначение |
| --- | --- |
| `env` | `local` — текстовые DEBUG-логи; `dev` — JSON DEBUG; `prod` — JSON INFO |
| `postgres_dsn` | Строка подключения к PostgreSQL |
| `sso_address` | Адрес gRPC-сервиса SSO |
| `jwt_secret` | Секрет проверки HMAC-подписи JWT |
| `redis_url` | URL подключения к Redis |
| `redis_ttl` | Срок хранения результата проверки участия, например `5m` |
| `grpc.port` | TCP-порт gRPC-сервера |
| `grpc.timeout` | Поле конфигурации; в текущем коде не применяется к серверу |

Путь к YAML передаётся флагом `--config` либо переменной окружения `CONFIG_PATH`. Флаг имеет приоритет. Автоматическое переопределение перечисленных YAML-полей одноимёнными переменными окружения в конфигурации не задано.

### 4. Миграции

Дождитесь готовности PostgreSQL и выполните из корня проекта:

```bash
go run ./cmd/migrator \
  -db-url="postgres://chat:chat@localhost:5432/chat?sslmode=disable" \
  -migrations-path="./migrations" \
  -action="up"
```

Миграция создаёт таблицы `chats`, `chat_members`, `messages` и индекс по `messages.chat_id`. Удаление чата каскадно удаляет его сообщения и участников.

Для отката используется `-action="down"`. Эта команда откатывает **все** миграции; текущая down-миграция удаляет таблицы вместе с данными.

### 5. Запуск сервиса

```bash
go run ./cmd/chat --config=./config/local.yaml
```

Или через переменную окружения:

```bash
CONFIG_PATH=./config/local.yaml go run ./cmd/chat
```

PostgreSQL и Redis должны быть доступны при старте: приложение проверяет соединения через `Ping`. SSO необходим для создания чатов. При получении `SIGINT` или `SIGTERM` сервер вызывает `GracefulStop`.

### Команды Task

`Taskfile.yaml` загружает `.env`. Чтобы использовать задачи миграций, создайте в корне проекта `.env`:

```dotenv
MIGRATOR_DB_URL=postgres://chat:chat@localhost:5432/chat?sslmode=disable
```

| Команда | Действие |
| --- | --- |
| `task` или `task run` | Запуск с `./config/local.yaml` |
| `task migrate-up` | Применение миграций |
| `task migrate-down` | Откат всех миграций |

При прямом запуске через `go run` файл `.env` автоматически не загружается.

## gRPC API

Сервис: `chat.Chat`. Контракт: [`proto/chat/chat.proto`](https://github.com/Ivan-Top-Pudge/mymsg-protos/blob/v0.0.1/proto/chat/chat.proto).

Все методы требуют metadata:

```text
authorization: Bearer <JWT>
```

JWT должен иметь HMAC-подпись с ключом `jwt_secret` и числовой claim `uid`. Идентификатор отправителя и пользователя, запрашивающего историю или удаление сообщения, берётся из токена. Поле `sender_id`, присутствующее в protobuf-запросе `SendMessage`, обработчик не использует для определения отправителя.

| Метод | Параметры запроса | Результат и поведение |
| --- | --- | --- |
| `CreateChat` | `members` | Возвращает `chat_id`; минимум два элемента, пользователи проверяются через SSO |
| `DeleteChat` | `chat_id` | Возвращает `deleted_chat_id`; удаляет чат и связанные данные |
| `SendMessage` | `chat_id`, `text` | Возвращает `msg_id`; отправитель должен состоять в чате |
| `DeleteMessage` | `chat_id`, `msg_id` | Возвращает `success`; удалить сообщение может его автор |
| `GetChatHistory` | `chat_id`, `limit`, `offset` | Возвращает `messages`; история доступна участнику чата |

Идентификаторы чата и сообщения должны быть положительными, текст сообщения — непустым, `limit > 0`, `offset >= 0`. Передавайте уникальные ID участников: повторяющиеся значения конфликтуют с составным первичным ключом `chat_members`. Создатель не добавляется в чат автоматически — включите его ID в `members`.

В истории каждое сообщение содержит `id`, `chat_id`, `sender_id`, `text` и `created_at` — Unix timestamp в секундах.

### Примеры с grpcurl

Reflection в сервере не зарегистрирован, поэтому grpcurl нужно передать `.proto`. В отдельном терминале получите контракт той же версии:

```bash
git clone --depth 1 --branch v0.0.1 \
  https://github.com/Ivan-Top-Pudge/mymsg-protos.git ../mymsg-protos

export CHAT_TOKEN="<JWT, выданный вашим SSO>"
```

Создание чата: пользователи `1` и `2` должны существовать в SSO.

```bash
grpcurl -plaintext \
  -import-path ../mymsg-protos/proto \
  -proto chat/chat.proto \
  -H "authorization: Bearer ${CHAT_TOKEN}" \
  -d '{"members":["1","2"]}' \
  localhost:44045 chat.Chat/CreateChat
```

Отправка сообщения и получение истории — с токеном участника чата. Замените `chat_id` на ID из ответа `CreateChat`:

```bash
grpcurl -plaintext \
  -import-path ../mymsg-protos/proto \
  -proto chat/chat.proto \
  -H "authorization: Bearer ${CHAT_TOKEN}" \
  -d '{"chat_id":"1","text":"Привет!"}' \
  localhost:44045 chat.Chat/SendMessage

grpcurl -plaintext \
  -import-path ../mymsg-protos/proto \
  -proto chat/chat.proto \
  -H "authorization: Bearer ${CHAT_TOKEN}" \
  -d '{"chat_id":"1","limit":"20","offset":"0"}' \
  localhost:44045 chat.Chat/GetChatHistory
```

### Особенности текущей реализации

- `DeleteChat` требует валидный JWT, но не проверяет участие пользователя в чате или отдельную роль для удаления.
- Отказ в доступе в `DeleteMessage` возвращается как `PermissionDenied`. Аналогичный отказ в `SendMessage` и `GetChatHistory` сейчас преобразуется обработчиками в `Internal`.
- Redis кэширует участие только для отправки сообщений. При получении истории участие проверяется непосредственно в PostgreSQL.
- gRPC-сервер и исходящее соединение к SSO настроены без TLS. Примеры выше предназначены для локального запуска.
- При старте в лог передаётся весь объект конфигурации, включая секрет и строки подключения. Учитывайте это при настройке окружения и доступа к логам.

## Тестирование и сборка

Модульные тесты сервисного слоя и JWT-interceptor не требуют Docker или запущенных внешних сервисов:

```bash
go test -v ./internal/services/chat ./internal/delivery/grpc/interceptors
```

Для полного набора тестов нужен работающий Docker: пакет PostgreSQL самостоятельно поднимает контейнер `postgres:17` через Testcontainers и применяет миграции.

```bash
go test -v ./...
```

Сборка сервиса и мигратора:

```bash
mkdir -p bin
go build -o bin/chat ./cmd/chat
go build -o bin/migrator ./cmd/migrator
```

GitHub Actions запускает golangci-lint, тесты и сборку при push и pull request в `main` и `master`.

## Структура проекта

| Путь | Назначение |
| --- | --- |
| `cmd/chat` | Точка входа gRPC-сервиса |
| `cmd/migrator` | CLI для SQL-миграций |
| `internal/app` | Сборка приложения и подключение зависимостей |
| `internal/app/grpc` | Запуск и остановка gRPC-сервера |
| `internal/config` | Загрузка конфигурации |
| `internal/clients/sso/grpc` | gRPC-клиент SSO |
| `internal/delivery/grpc/interceptors` | JWT-аутентификация и ID пользователя в контексте |
| `internal/grpc/chat` | Обработчики RPC и валидация запросов |
| `internal/services/chat` | Бизнес-логика, интерфейсы зависимостей, моки и тесты |
| `internal/domain/models` | Доменная модель сообщения |
| `internal/storage/postgres` | Хранилище и интеграционные тесты |
| `internal/storage/redis` | Кэш проверок участия |
| `migrations` | SQL-миграции |
| `Taskfile.yaml` | Задачи запуска и миграций |
| `.github/workflows/ci.yml` | CI-пайплайн |
