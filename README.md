# Corp VPN Bot

Telegram-бот для корпоративного VPN на базе Remnawave.

## Логика

- **Админ** добавляет сотрудников по их Telegram **username** (без `@`)
- **Сотрудник** пишет `/start`, бот сверяет его username с базой и выдаёт ссылку подписки
- Если username не найден — бот отвечает «обратитесь к администратору»

## Структура

```
.
├── cmd/bot/             # main + embed schema.sql
├── internal/
│   ├── config/          # чтение env
│   ├── db/              # pgx pool, методы работы с сотрудниками
│   ├── remnawave/       # клиент API
│   └── handlers/        # обработчики команд + state-машина
├── migrations/          # SQL-схема (применяется автоматически при старте)
├── Dockerfile
├── docker-compose.yml
└── .env.example
```

## Развёртывание

Бот рассчитан на запуск **на том же хосте, где Remnawave**, и подключение
к **той же сети и Postgres**, что у панели.

### 1. Узнайте имя docker-сети Remnawave

```bash
docker network ls | grep -i remnawave
```

Скопируйте имя (например, `remnawave_default`) и подставьте в `docker-compose.yml`,
в секцию `networks.remnawave-net.name`.

### 2. Узнайте имя контейнера Postgres

```bash
docker ps --format '{{.Names}}' | grep -iE 'db|postgres'
```

Скопируйте имя (например, `remnawave-db`) — это будет хост в `DATABASE_URL`.

### 3. Узнайте имя контейнера Remnawave-backend

```bash
docker ps --format 'table {{.Names}}\t{{.Ports}}'
```

Скопируйте имя сервиса backend Remnawave — это будет хост в `REMNAWAVE_URL`
(обычно `http://remnawave:3000`).

### 4. Получите API-токен Remnawave

Панель → **Settings → API Tokens** → создайте новый, скопируйте.

### 5. Получите UUID Internal Squad

Панель → **Internal Squads** → откройте нужную группу → скопируйте UUID
из адресной строки или из деталей.

### 6. Заполните `.env`

```bash
cp .env.example .env
nano .env
```

### 7. Запуск

```bash
docker compose up -d --build
docker logs -f corp-vpn-bot
```

При старте бот сам применит SQL-миграцию (создаст таблицу `bot_employees`
в базе Remnawave; чужие таблицы он не трогает).

## Команды бота

### Админ

| Команда                          | Действие                                   |
|----------------------------------|--------------------------------------------|
| `/list`                          | Показать список сотрудников                |
| `/add username [username2 ...]`  | Добавить одного или нескольких через пробел|
| `/addlist`                       | Ответ — список юзернеймов по одному на строку |
| `/del username`                  | Удалить сотрудника (и из Remnawave, и из БД) |
| `/cancel`                        | Отменить ожидание ввода списка             |
| `/help`                          | Справка                                    |

`/addlist` запускает диалог: после команды отправьте список одним сообщением,
каждый username с новой строки. С `@` или без — нормализуется автоматически.

### Сотрудник

| Команда  | Действие |
|----------|----------|
| `/start` | Получить ссылку подписки, если добавлен админом |

## Параметры подписки

В `.env`:
- `SUB_DAYS` — срок действия (по умолчанию 365)
- `SUB_DEVICES` — лимит устройств (5)
- `SUB_TRAFFIC_GB` — лимит трафика в ГБ; `0` = безлимит

## Безопасность

- Бот хранит только `username`, UUID в Remnawave и саму ссылку подписки.
  Никаких паролей сотрудников бот не знает и не запрашивает.
- Telegram username регистронезависим, в БД хранится в нижнем регистре.
- Если сотрудник сменит username в Telegram, ему нужно сообщить админу —
  старый username перестанет работать. Удалите его командой `/del` и добавьте заново.
- Docker-образ — distroless с пользователем `nonroot`, без shell.

## Отладка

```bash
docker logs -f corp-vpn-bot
docker exec -it corp-vpn-bot /app/bot --help  # не сработает, distroless
```

Проверить, что бот видит Postgres:
```bash
docker exec -it remnawave-db psql -U postgres -d postgres -c '\dt bot_employees'
```

Проверить, что бот видит Remnawave:
```bash
docker exec corp-vpn-bot wget -qO- http://remnawave:3000/  # не сработает в distroless
# вместо этого смотри логи бота — при первой попытке /add будет видна ошибка
```

## Обновление

```bash
git pull
docker compose up -d --build
```

Миграции применяются автоматически и идемпотентны.
# tg-bot-remna
