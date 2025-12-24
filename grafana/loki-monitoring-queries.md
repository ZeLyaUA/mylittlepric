# Loki Monitoring Queries - Поиск проблем без session_id

## Общий мониторинг ошибок

### 1. Все ошибки за последний час
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | line_format "{{.time}} [{{.session_id}}] {{.user_id}} - {{.msg}}: {{.error}}"
```

**Что увидите:**
- Все ошибки с session_id и user_id
- Можно скопировать session_id для детального анализа

### 2. Группировка ошибок по типу
```logql
sum by (msg) (
  count_over_time(
    {compose_service="mylittleprice-backend"}
      | json
      | level = "ERROR" [1h]
  )
)
```

**Покажет:**
- Топ ошибок за последний час
- Какая ошибка встречается чаще всего

### 3. Последние 20 ошибок с контекстом
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | line_format "{{.time}} | Session: {{.session_id}} | User: {{.user_id}} | {{.msg}} | Error: {{.error}}"
```

## Поиск проблем с SERP API

### 4. Все неудачные SERP запросы
```logql
{compose_service="mylittleprice-backend"}
  |~ "SERP API error|No relevant products"
  | json
  | line_format "{{.time}} | Session: {{.session_id}} | Query: '{{.query}}' | Error: {{.error}}"
```

**Используйте для:**
- Найти какие запросы не работают
- Увидеть session_id для детального анализа

### 5. SERP quota ошибки
```logql
{compose_service="mylittleprice-backend"}
  |= "Quota error detected"
  | json
  | line_format "{{.time}} | Key: {{.key_index}} | Session: {{.session_id}}"
```

**Покажет:**
- Какие API ключи исчерпались
- Время когда это произошло

### 6. Медленные SERP запросы (>3 секунд)
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP response received"
  | json
  | duration_seconds > 3
  | line_format "{{.time}} | Session: {{.session_id}} | Query: {{.query}} | Duration: {{.duration_seconds}}s | Key: {{.key_index}}"
```

## Поиск проблем конкретного пользователя

### 7. Все ошибки конкретного user_id
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | user_id = "USER_ID_HERE"
```

### 8. Все сессии конкретного пользователя
```logql
{compose_service="mylittleprice-backend"}
  | json
  | user_id = "USER_ID_HERE"
  | session_id != ""
  | line_format "Session: {{.session_id}}"
```

**Выведет все session_id пользователя для дальнейшего анализа**

## Мониторинг WebSocket проблем

### 9. WebSocket отключения и ошибки
```logql
{compose_service="mylittleprice-backend"}
  |~ "WebSocket (error|timeout|disconnected)"
  | line_format "{{.time}} | {{.msg}}"
```

### 10. WebSocket таймауты (нет ping)
```logql
{compose_service="mylittleprice-backend"}
  |= "WebSocket timeout (no ping received)"
  | line_format "{{.time}} | Client timeout"
```

## Анализ производительности

### 11. Самые медленные операции
```logql
{compose_service="mylittleprice-backend"}
  |~ "duration_seconds"
  | json
  | duration_seconds > 2
  | line_format "{{.time}} | {{.msg}} | Duration: {{.duration_seconds}}s | Session: {{.session_id}}"
```

### 12. Статистика времени SERP запросов
```logql
quantile_over_time(0.95,
  {compose_service="mylittleprice-backend"}
    |= "SERP response received"
    | json
    | unwrap duration_seconds [5m]
)
```

**Показывает 95-й перцентиль времени ответа**

## Поиск по содержимому запроса

### 13. Поиск по тексту запроса
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | query =~ ".*iphone.*"
  | line_format "{{.time}} | Session: {{.session_id}} | Query: '{{.query}}' | Country: {{.country}}"
```

**Используйте для:**
- Найти все поиски по конкретному продукту
- Увидеть session_id для детального анализа

### 14. Поиск без результатов
```logql
{compose_service="mylittleprice-backend"}
  |= "No relevant products found"
  | json
  | line_format "{{.time}} | Session: {{.session_id}} | Failed Query: '{{.query}}' | Score: {{.relevance_score}}"
```

**Покажет:**
- Какие запросы не находят товары
- Session_id для проверки почему

## Мониторинг пользовательской активности

### 15. Активные сессии (последние сообщения)
```logql
{compose_service="mylittleprice-backend"}
  |= "message processing completed"
  | json
  | __timestamp__ > now() - 5m
  | line_format "{{.time}} | Session: {{.session_id}} | User: {{.user_id}} | Status: {{.status}}"
```

**Показывает активных пользователей за последние 5 минут**

### 16. Анонимные vs авторизованные запросы
```logql
# Анонимные
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | user_id = ""

# Авторизованные
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | user_id != ""
```

## Топ проблемных запросов

### 17. Топ-10 запросов с ошибками
```logql
topk(10,
  sum by (query) (
    count_over_time(
      {compose_service="mylittleprice-backend"}
        |= "SERP API error"
        | json [1h]
    )
  )
)
```

**Покажет какие запросы чаще всего вызывают ошибки**

### 18. Топ-10 медленных запросов
```logql
topk(10,
  avg by (query) (
    avg_over_time(
      {compose_service="mylittleprice-backend"}
        |= "SERP response received"
        | json
        | unwrap duration_seconds [1h]
    )
  )
)
```

## Поиск по времени

### 19. Ошибки в определенное время
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | __timestamp__ >= 1638360000000
  | __timestamp__ <= 1638363600000
  | line_format "{{.time}} | Session: {{.session_id}} | {{.msg}}"
```

**Используйте Time Range в Grafana вместо timestamp фильтров**

### 20. Всплески ошибок
```logql
rate(
  {compose_service="mylittleprice-backend"}
    | json
    | level = "ERROR" [1m]
)
```

**График покажет когда было больше всего ошибок**

## Практические сценарии

### Сценарий 1: "Пользователь жалуется на ошибку"

1. **Найдите все недавние ошибки:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | __timestamp__ > now() - 1h
  | line_format "{{.session_id}} | {{.user_id}} | {{.msg}}"
```

2. **Скопируйте session_id или user_id**

3. **Детальный анализ:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "COPIED_SESSION_ID"
```

### Сценарий 2: "Поиск не работает для определенных товаров"

1. **Найдите неудачные поиски:**
```logql
{compose_service="mylittleprice-backend"}
  |= "No relevant products found"
  | json
  | line_format "Query: '{{.query}}' | Session: {{.session_id}}"
```

2. **Проверьте один из запросов детально:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "COPIED_SESSION_ID"
  |~ "SERP"
```

### Сценарий 3: "Система медленная"

1. **Найдите медленные операции:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | duration_seconds > 3
  | line_format "{{.msg}} | {{.duration_seconds}}s | Session: {{.session_id}}"
```

2. **Анализируйте конкретную сессию:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "COPIED_SESSION_ID"
```

### Сценарий 4: "API ключи не работают"

```logql
{compose_service="mylittleprice-backend"}
  |~ "Quota error|key.*exhausted"
  | json
  | line_format "{{.time}} | Key: {{.key_index}} | {{.msg}}"
```

## Алерты (для настройки в Grafana)

### Алерт 1: Много ошибок
```logql
sum(
  rate(
    {compose_service="mylittleprice-backend"}
      | json
      | level = "ERROR" [5m]
  )
) > 10
```

**Срабатывает:** Если больше 10 ошибок в минуту

### Алерт 2: Все API ключи исчерпаны
```logql
sum(
  count_over_time(
    {compose_service="mylittleprice-backend"}
      |= "Quota error detected" [5m]
  )
) > 5
```

**Срабатывает:** Если 5+ quota ошибок за 5 минут

### Алерт 3: Медленные запросы
```logql
quantile_over_time(0.95,
  {compose_service="mylittleprice-backend"}
    |= "SERP response received"
    | json
    | unwrap duration_seconds [5m]
) > 5
```

**Срабатывает:** Если 95% запросов медленнее 5 секунд

## Советы

### 🎯 Эффективный workflow

1. **Начните с общего обзора** (запрос #1 - все ошибки)
2. **Найдите session_id** в результатах
3. **Детальный анализ** конкретной сессии
4. **Проверьте паттерн** - есть ли похожие проблемы?

### 📊 Дашборды

Создайте дашборд в Grafana с панелями:
1. **Error Rate** (график ошибок во времени)
2. **Top Errors** (топ ошибок)
3. **Recent Errors** (последние ошибки с session_id)
4. **SERP Performance** (время ответа SERP)
5. **WebSocket Status** (подключения и отключения)

### 🔍 Быстрый поиск

Используйте **Live Tail** в Grafana для мониторинга в реальном времени:
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level =~ "ERROR|WARN"
```

### 💡 Полезные фильтры

- `| level = "ERROR"` - только ошибки
- `| level =~ "ERROR|WARN"` - ошибки и предупреждения
- `| session_id != ""` - только с session_id
- `| user_id != ""` - только авторизованные
- `| duration_seconds > 2` - медленные операции
- `|~ "SERP|Gemini"` - только AI/Search операции
