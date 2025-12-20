# WebSocket Reconnection Fix

## Проблема

WebSocket соединение отключалось (код 1006 - abnormal closure) и **НЕ переподключалось** для анонимных пользователей.

### Симптомы
```
💓 Sending ping
🔌 WebSocket closed: 1006
💔 Stopping heartbeat ping
🔄 WebSocket reconnection check
🔌 Not reconnecting - user logged out (no refresh token)
```

### Причина

Старая логика в `shouldReconnect`:
```typescript
// ❌ ПЛОХО - блокирует переподключение для анонимных пользователей
if (!refreshToken) {
  return false;  // Не переподключаемся!
}
```

**Проблема:** Анонимные пользователи (без логина) не имеют `refreshToken`, поэтому система считала что они вышли из системы и не пыталась переподключиться.

## Решение

### Исправленная логика

Файл: `frontend/src/features/chat/hooks/use-chat.ts`

```typescript
shouldReconnect: (closeEvent) => {
  console.log("🔄 WebSocket reconnection check:", {
    code: closeEvent?.code,
    reason: closeEvent?.reason,
    hasRefreshToken: !!refreshToken,
    hasAccessToken: !!accessToken,
    isExpired: accessToken ? isTokenExpired() : false,
  });

  // For authenticated users - check if token is expired
  if (accessToken && refreshToken) {
    // If token is expired, refresh before reconnecting
    if (isTokenExpired()) {
      console.log("🔐 Token expired, refreshing before reconnect...");
      import('@/shared/lib/auth-api').then(({ authAPI }) => {
        authAPI.refreshAccessToken().catch((error) => {
          console.error("❌ Failed to refresh token on reconnect:", error);
          useAuthStore.getState().clearAuth();
        });
      });
      // Don't reconnect immediately, wait for token refresh to trigger new connection
      return false;
    }
  }

  // ✅ НОВОЕ: Всегда разрешаем переподключение
  // Блокируем только при нормальном закрытии (код 1000 - пользователь сам закрыл)
  if (closeEvent?.code === 1000) {
    console.log("🔌 Not reconnecting - normal closure (user action)");
    return false;
  }

  console.log("✅ Will attempt to reconnect");
  return true;  // ✅ Всегда переподключаемся!
},
```

### Что изменилось

1. **✅ Анонимные пользователи переподключаются** автоматически
2. **✅ Авторизованные пользователи** проверяют токен перед переподключением
3. **✅ Блокируется только** намеренное закрытие (код 1000)
4. **✅ Любые ошибки** (1006, network errors) → автоматическое переподключение

### Коды закрытия WebSocket

| Код | Описание | Действие |
|-----|----------|----------|
| 1000 | Normal Closure | ❌ Не переподключаемся (пользователь сам закрыл) |
| 1006 | Abnormal Closure | ✅ Переподключаемся (сетевая ошибка, таймаут) |
| 1001 | Going Away | ✅ Переподключаемся (страница перезагружается) |
| 1011 | Internal Error | ✅ Переподключаемся (ошибка сервера) |

## Heartbeat (Ping/Pong)

### Frontend
- **Отправляет ping** каждые 30 секунд
- **Автоматически** при активном соединении
- **Останавливается** при отключении

```typescript
useEffect(() => {
  if (!isConnected) return;

  const pingInterval = setInterval(() => {
    if (readyState === ReadyState.OPEN) {
      console.log("💓 Sending ping");
      sendJsonMessage({ type: 'ping' });
    }
  }, 30000); // 30 seconds

  return () => clearInterval(pingInterval);
}, [isConnected, readyState, sendJsonMessage]);
```

### Backend
- **Ожидает сообщение** до 60 секунд
- **Сбрасывает таймер** при любом сообщении
- **Отвечает pong** на ping

```go
// Set read deadline
c.SetReadDeadline(time.Now().Add(60 * time.Second))

// Reset on any message
c.SetReadDeadline(time.Now().Add(60 * time.Second))
```

## Настройки переподключения

```typescript
export function useChat(options: UseChatOptions = {}): UseChatReturn {
  const {
    reconnectAttempts = 10,      // 10 попыток
    reconnectInterval = 3000,    // 3 секунды между попытками
  } = options;
```

### Временная линия при ошибке

```
0s  - Соединение разорвано (код 1006)
0s  - shouldReconnect() → return true
3s  - Попытка #1
6s  - Попытка #2
9s  - Попытка #3
...
30s - Попытка #10 (последняя)
```

## Что делать если всё равно отключается

### 1. Проверьте консоль браузера

**Должны видеть:**
```
💓 Sending ping        (каждые 30 секунд)
✅ WebSocket connected
🔄 WebSocket reconnection check
✅ Will attempt to reconnect
```

**Не должны видеть:**
```
🔌 Not reconnecting - user logged out (no refresh token)  ❌ ПЛОХО
```

### 2. Проверьте backend логи

**Должны видеть:**
```json
{"msg":"🔌 Client connected","time":"..."}
```

**Не должны видеть частые:**
```json
{"msg":"⏱️ WebSocket timeout (no ping received)"}  ❌ Ping не доходит
```

### 3. Проверьте reverse proxy / load balancer

Если используете nginx/traefik/cloudflare:

**nginx:**
```nginx
location /ws {
    proxy_pass http://backend;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;  # ← Увеличьте timeout!
    proxy_send_timeout 3600s;
}
```

**Cloudflare:**
- WebSocket timeout: 100 секунд (нельзя изменить на Free плане)
- Используйте другой CDN или Enterprise план

### 4. Сетевые проблемы

**Проверьте:**
- WiFi не отключается?
- VPN стабилен?
- Мобильный интернет не переключается между вышками?

**Решение:**
- Heartbeat ping (уже реализован) ✅
- Меньший интервал ping (15-20 секунд вместо 30)

## Как уменьшить интервал ping

Если всё равно отключается, попробуйте более частый ping:

```typescript
const pingInterval = setInterval(() => {
  if (readyState === ReadyState.OPEN) {
    console.log("💓 Sending ping");
    sendJsonMessage({ type: 'ping' });
  }
}, 20000); // ← Изменить на 20 секунд
```

И на backend:
```go
// Уменьшить read deadline
c.SetReadDeadline(time.Now().Add(40 * time.Second))  // 40 вместо 60
```

## Debug

### Проверить что переподключение работает

1. **Откройте консоль** браузера
2. **Искусственно разорвите** соединение:
   ```javascript
   // В консоли браузера
   window.location.reload()  // Или закройте/откройте вкладку
   ```
3. **Должны увидеть:**
   ```
   🔌 WebSocket closed: 1001
   🔄 WebSocket reconnection check
   ✅ Will attempt to reconnect
   ✅ WebSocket connected  // Через 3 секунды
   ```

### Логи для отладки

**Включите verbose логи:**
```typescript
// В use-chat.ts
console.log("💓 Heartbeat state:", {
  isConnected,
  readyState,
  lastPingAt: new Date().toISOString()
});
```

**Проверьте backend:**
```bash
# Смотрите WebSocket логи
docker-compose logs -f backend | grep -i websocket
```

## Итог

- ✅ **Анонимные пользователи** теперь переподключаются автоматически
- ✅ **Heartbeat ping/pong** поддерживает соединение активным
- ✅ **10 попыток переподключения** по 3 секунды
- ✅ **Логирование** для отладки

**Результат:** WebSocket должен оставаться активным бесконечно для всех пользователей! 🎉
