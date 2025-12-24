# Быстрая отладка - Шпаргалка

## 🚨 Пользователь сообщил об ошибке

### Шаг 1: Найдите его session_id

**Grafana** → **Explore** → **Loki**

```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | __timestamp__ > now() - 1h
  | line_format "{{.time}} | Session: {{.session_id}} | User: {{.user_id}} | {{.msg}}"
```

✅ Скопируйте `session_id` или `user_id` из результатов

### Шаг 2: Посмотрите что произошло

```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "ВСТАВЬТЕ_SESSION_ID"
```

✅ Увидите весь путь пользователя: все запросы, ошибки, таймлайн

## 🔍 Типовые проблемы

### "Поиск не работает"

```logql
{compose_service="mylittleprice-backend"}
  |= "No relevant products found"
  | json
  | __timestamp__ > now() - 1h
  | line_format "Session: {{.session_id}} | Query: '{{.query}}'"
```

Копируйте session_id → анализируйте детально

### "Все медленно"

```logql
{compose_service="mylittleprice-backend"}
  | json
  | duration_seconds > 3
  | line_format "{{.time}} | {{.msg}} | {{.duration_seconds}}s | Session: {{.session_id}}"
```

### "Quota ошибки"

```logql
{compose_service="mylittleprice-backend"}
  |= "Quota error detected"
  | json
  | line_format "{{.time}} | Key: {{.key_index}}"
```

Проверьте: какие ключи исчерпаны, добавьте новые в `.env`

### "WebSocket отключается"

```logql
{compose_service="mylittleprice-backend"}
  |~ "WebSocket (timeout|error)"
  | line_format "{{.time}} | {{.msg}}"
```

Должны видеть `💓 Sending ping` в консоли браузера каждые 30 секунд

## 📊 Мониторинг

### Топ ошибок

```logql
topk(10,
  sum by (msg) (
    count_over_time(
      {compose_service="mylittleprice-backend"}
        | json
        | level = "ERROR" [1h]
    )
  )
)
```

### Активные пользователи

```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | __timestamp__ > now() - 5m
  | line_format "{{.time}} | Session: {{.session_id}}"
```

### График ошибок

```logql
rate(
  {compose_service="mylittleprice-backend"}
    | json
    | level = "ERROR" [1m]
)
```

## 🛠️ Live отладка

### Real-time мониторинг

**Grafana** → **Live** (кнопка справа сверху)

```logql
{compose_service="mylittleprice-backend"}
  | json
  | level =~ "ERROR|WARN"
```

✅ Видите ошибки в реальном времени

### Следите за конкретной сессией

```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "abc-123"
```

+ включите **Live** → видите логи в реальном времени

## 📋 Чеклист проблем

### WebSocket

- [ ] Видите `💓 Sending ping` в консоли браузера?
- [ ] Видите `pong` ответы от сервера?
- [ ] Таймаут больше 60 секунд?

### SERP API

- [ ] Есть ли quota ошибки в логах?
- [ ] Проверили все API ключи?
- [ ] Время ответа нормальное (<3s)?

### Логирование

- [ ] LOG_FORMAT=json в .env?
- [ ] Promtail запущен?
- [ ] Видите логи в Grafana?

## 💡 Полезные команды

### Проверить Loki

```bash
docker-compose -f docker-compose.monitoring.yml ps loki
docker-compose -f docker-compose.monitoring.yml logs loki
```

### Проверить Promtail

```bash
docker-compose -f docker-compose.monitoring.yml ps promtail
docker-compose -f docker-compose.monitoring.yml logs promtail
```

### Перезапустить мониторинг

```bash
docker-compose -f docker-compose.monitoring.yml restart
```

## 📖 Больше информации

- **Полное руководство:** `docs/LOGGING_RU.md`
- **Все запросы:** `grafana/loki-monitoring-queries.md`
- **Детали изменений:** `docs/CHANGES_SUMMARY.md`
