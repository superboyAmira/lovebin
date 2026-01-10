# LoveBin Production Deployment

Этот каталог содержит скрипты и конфигурации для развертывания LoveBin в production окружении.

## Структура

```
deploy/
├── nginx/              # Конфигурация Nginx
│   ├── nginx.conf      # Основной конфигурационный файл
│   └── conf.d/         # Конфигурации серверов
│       └── lovebin.conf # Конфигурация для LoveBin
├── setup.sh            # Скрипт первоначальной настройки сервера
├── deploy.sh           # Скрипт развертывания приложения
└── README.md           # Этот файл
```

## Требования

- Ubuntu 20.04+ или Debian 11+
- Docker и Docker Compose
- Root доступ для первоначальной настройки

## Быстрый старт

### 1. Первоначальная настройка сервера

```bash
sudo bash deploy/setup.sh
```

Этот скрипт:
- Обновит систему
- Установит Docker, Docker Compose и необходимые пакеты
- Настроит firewall (UFW)
- Настроит fail2ban для защиты от брутфорса
- Создаст директорию для приложения

### 2. Настройка конфигурации

1. Скопируйте `config/.env.example` в `config/.env`
2. Обновите следующие значения:
   - `DOCKER_IMAGE` - путь к вашему образу в GitHub Container Registry
   - `POSTGRES_PASSWORD` - сильный пароль для PostgreSQL
   - `MINIO_ROOT_PASSWORD` - сильный пароль для MinIO
   - `S3_ACCESS_KEY_ID` и `S3_SECRET_ACCESS_KEY` - ключи для S3

### 3. Развертывание

```bash
bash deploy/deploy.sh
```

Этот скрипт:
- Загрузит последний образ из registry
- Остановит существующие контейнеры
- Запустит новые контейнеры
- Проверит здоровье приложения

## Nginx Rate Limiting

Nginx настроен с rate limiting для защиты от злоупотреблений:

- **Лимит загрузки**: 3 запроса в минуту на `/upload`
- **Бан**: 5 минут после превышения лимита
- **Статус**: 429 Too Many Requests при превышении лимита

## Мониторинг

### Просмотр логов

```bash
# Все сервисы
cd deploy
docker-compose -f docker-compose.prod.yml logs -f

# Только приложение
docker-compose -f docker-compose.prod.yml logs -f app

# Только Nginx
docker-compose -f docker-compose.prod.yml logs -f nginx
```

### Проверка статуса

```bash
cd deploy
docker-compose -f docker-compose.prod.yml ps
```

### Проверка здоровья

```bash
curl http://localhost/health
```

## Обновление

Для обновления приложения:

1. Убедитесь, что новый образ загружен в registry
2. Обновите `DOCKER_TAG` в `config/.env` (или используйте `latest`)
3. Запустите `deploy/deploy.sh`

## Безопасность

- Все пароли должны быть сложными (минимум 32 символа)
- Firewall настроен на разрешение только необходимых портов
- fail2ban защищает от брутфорса SSH
- Nginx скрывает версию и добавляет security headers
- Rate limiting защищает от злоупотреблений

## Troubleshooting

### Проблемы с подключением к базе данных

Проверьте логи PostgreSQL:
```bash
cd deploy
docker-compose -f docker-compose.prod.yml logs postgres
```

### Проблемы с S3/MinIO

Проверьте логи MinIO:
```bash
cd deploy
docker-compose -f docker-compose.prod.yml logs minio
```

### Проблемы с Nginx

Проверьте конфигурацию:
```bash
docker exec lovebin-nginx-prod nginx -t
```

Просмотрите логи:
```bash
cd deploy
docker-compose -f docker-compose.prod.yml logs nginx
```
