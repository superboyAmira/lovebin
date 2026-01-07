# Инструкция по настройке LoveBin

## Предварительные требования

1. Go 1.21 или выше
2. PostgreSQL 12+
3. S3-совместимое хранилище (AWS S3, MinIO и т.д.)
4. sqlc для генерации кода из SQL запросов

## Установка зависимостей

```bash
go mod download
```

## Установка sqlc

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

Или через Homebrew (macOS):
```bash
brew install sqlc
```

## Настройка базы данных

1. Создайте базу данных:
```bash
createdb lovebin
```

2. Примените миграции:
```bash
psql -U postgres -d lovebin -f migrations/001_init.sql
```

## Генерация кода sqlc

После создания SQL запросов необходимо сгенерировать Go код:

```bash
# Для media-service
cd internal/services/media-service/repository
sqlc generate
cd ../../..

# Для access-service
cd internal/services/access-service/repository
sqlc generate
cd ../../..
```

Или используйте Makefile:
```bash
make sqlc
```

## Настройка переменных окружения

Создайте файл `.env` на основе `.env.example`:

```bash
cp .env.example .env
```

Заполните необходимые значения:
- Настройки PostgreSQL
- Настройки S3 (или MinIO)
- Настройки сервера

## Запуск приложения

```bash
go run cmd/lovebin/main.go
```

Или используйте Makefile:
```bash
make run
```

## Использование API

### Загрузка медиа

```bash
curl -X POST http://localhost:8080/upload \
  -F "file=@photo.jpg" \
  -F "password=mysecret" \
  -F "expires_in=24h"
```

Ответ:
```json
{
  "resource_key": "abc123...#encryption_key",
  "url": "/media/abc123...#encryption_key"
}
```

**Важно**: Сохраните полный URL с encryption key - он нужен для скачивания!

### Скачивание медиа

```bash
# С паролем
curl -X GET "http://localhost:8080/media/abc123...#encryption_key?password=mysecret" \
  --output downloaded.jpg

# Без пароля (если не был установлен при загрузке)
curl -X GET "http://localhost:8080/media/abc123...#encryption_key" \
  --output downloaded.jpg
```

**Важно**: Ресурс будет удален после первого успешного просмотра!

## Архитектура

### Модули (modules/)

Все внешние зависимости организованы в модулях с единым интерфейсом и методом `Init`:

- `modules/logger` - zap логгер
- `modules/postgres` - PostgreSQL клиент
- `modules/s3` - S3 клиент
- `modules/encryption` - криптографические функции

### Сервисы (internal/services/)

Бизнес-логика разделена на сервисы:

- `media-service` - загрузка и скачивание медиа
- `access-service` - управление доступом (пароли, проверка)

### Репозитории

Каждый сервис имеет свой репозиторий с SQL запросами для sqlc:
- `internal/services/media-service/repository/`
- `internal/services/access-service/repository/`

### Приложение (internal/app/)

Конструктор приложения инициализирует все модули и сервисы в правильном порядке.

## Безопасность

- Данные шифруются на стороне сервера перед сохранением в S3
- Ключ шифрования является частью URL (не хранится в БД)
- Пароли хешируются с помощью bcrypt
- Ресурсы автоматически удаляются после первого просмотра
- Поддержка истечения срока действия

## Разработка

После изменения SQL запросов:
1. Обновите `queries.sql` в соответствующем репозитории
2. Запустите `sqlc generate`
3. Обновите код в `repository.go` если необходимо

## Troubleshooting

### Ошибка подключения к PostgreSQL
Проверьте настройки в `.env` и убедитесь, что PostgreSQL запущен.

### Ошибка подключения к S3
- Проверьте credentials
- Для MinIO убедитесь, что указан правильный endpoint
- Проверьте права доступа к bucket

### Ошибка генерации sqlc
Убедитесь, что:
- sqlc установлен
- SQL запросы синтаксически корректны
- Пути в `sqlc.yaml` правильные

