# LoveBin Deployment Guide

## Подготовка образа для GitHub Container Registry

### 1. Настройка GitHub Container Registry

1. Создайте Personal Access Token (PAT) в GitHub:
   - Settings → Developer settings → Personal access tokens → Tokens (classic)
   - Выберите права: `write:packages`, `read:packages`, `delete:packages`

2. Войдите в Docker с вашим токеном:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

### 2. Сборка и загрузка образа

Используйте Makefile для автоматизации:

```bash
# Установите переменные окружения (опционально)
export GITHUB_USER=your-username
export VERSION=v1.0.0

# Соберите и загрузите образ
make docker-push
```

Или вручную:

```bash
# Соберите образ (используя Makefile)
make docker-build

# Или вручную:
docker build -t ghcr.io/USERNAME/lovebin:latest -f deploy/Dockerfile .

# Загрузите в registry
make docker-push

# Или вручную:
docker push ghcr.io/USERNAME/lovebin:latest
```

### 3. Обновление конфигурации

Обновите `config/.env`:
```env
DOCKER_IMAGE=ghcr.io/USERNAME/lovebin
DOCKER_TAG=latest
```

## Production развертывание

### 1. Первоначальная настройка сервера

```bash
sudo bash deploy/setup.sh
```

### 2. Настройка конфигурации

1. Скопируйте пример конфигурации:
```bash
cp config/.env.example config/.env
```

2. Обновите `config/.env`:
   - `DOCKER_IMAGE` - путь к вашему образу
   - `POSTGRES_PASSWORD` - сильный пароль (минимум 32 символа)
   - `MINIO_ROOT_PASSWORD` - сильный пароль
   - `S3_ACCESS_KEY_ID` и `S3_SECRET_ACCESS_KEY` - ключи доступа

3. **ВАЖНО**: Используйте сложные пароли! Генерируйте их с помощью:
```bash
python3 -c "import secrets; print(secrets.token_urlsafe(32))"
```

### 3. Развертывание

```bash
bash deploy/deploy.sh
```

Скрипт автоматически:
- Загрузит последний образ из registry
- Остановит старые контейнеры
- Запустит новые контейнеры
- Проверит здоровье приложения

## Nginx Rate Limiting

Nginx настроен с защитой от злоупотреблений:

- **Лимит**: 3 запроса в минуту на `/upload`
- **Бан**: После превышения лимита IP блокируется на 5 минут
- **Статус**: 429 Too Many Requests

## Мониторинг

### Логи

```bash
# Все сервисы
docker-compose -f docker-compose.prod.yml logs -f

# Конкретный сервис
docker-compose -f docker-compose.prod.yml logs -f app
docker-compose -f docker-compose.prod.yml logs -f nginx
```

### Статус контейнеров

```bash
docker-compose -f docker-compose.prod.yml ps
```

### Health check

```bash
curl http://localhost/health
```

## Обновление приложения

1. Загрузите новый образ в registry:
```bash
make docker-push
```

2. Обновите `DOCKER_TAG` в `config/.env` (или используйте `latest`)

3. Запустите развертывание:
```bash
bash deploy/deploy.sh
```

## Безопасность

- ✅ Все пароли хранятся в `config/.env` (не коммитится в git)
- ✅ Firewall настроен (только порты 22, 80, 443)
- ✅ fail2ban защищает от брутфорса SSH
- ✅ Nginx скрывает версию и добавляет security headers
- ✅ Rate limiting защищает от злоупотреблений
- ✅ Все сервисы работают в изолированной сети Docker

## Troubleshooting

### Проблемы с доступом к registry

Убедитесь, что:
- Вы авторизованы в Docker: `docker login ghcr.io`
- У токена есть права на запись пакетов
- Путь к образу правильный в `config/.env`

### Проблемы с подключением к базе данных

Проверьте логи:
```bash
cd deploy
docker-compose -f docker-compose.prod.yml logs postgres
```

Убедитесь, что пароль в `config/.env` совпадает с настройками PostgreSQL.

### Проблемы с Nginx

Проверьте конфигурацию:
```bash
docker exec lovebin-nginx-prod nginx -t
```

Просмотрите логи:
```bash
docker-compose -f docker-compose.prod.yml logs nginx
```
