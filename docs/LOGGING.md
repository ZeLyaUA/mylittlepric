# Structured Logging Guide

## Overview

MyLittlePrice uses structured logging with Go's `log/slog` package integrated with Grafana Loki for centralized log aggregation and analysis.

## Log Levels

- **DEBUG**: Detailed diagnostic information
- **INFO**: General informational messages (default)
- **WARN**: Warning messages for potential issues
- **ERROR**: Error messages for failures

Configure via `LOG_LEVEL` environment variable.

## Structured Fields

All logs automatically include contextual information:

### Automatic Context Fields
- `request_id`: Unique request identifier
- `user_id`: Authenticated user ID (if available)
- `session_id`: Chat session ID
- `time`: Timestamp
- `level`: Log level
- `msg`: Log message

### SERP API Logging

SERP requests automatically log:
- `query`: Search query
- `search_type`: Type of search (exact, broad, etc.)
- `country`: Country code
- `language`: Language code
- `min_price`, `max_price`: Price filters (if set)
- `duration_seconds`: Request duration
- `key_index`: API key used
- `product_count`: Number of products found
- `relevance_score`: Relevance score
- `top_products`: Top 3 product names
- `cache_key`: Cache key (for cached results)

## Example Log Output

### SERP Search Initiated
```json
{
  "time": "2025-11-28T09:45:12.123Z",
  "level": "INFO",
  "msg": "🔍 SERP search initiated",
  "session_id": "abc-123-def",
  "user_id": "user-456",
  "query": "iphone 15 pro",
  "search_type": "exact",
  "country": "US",
  "language": "en",
  "min_price": 500,
  "max_price": 1500
}
```

### SERP Search Completed
```json
{
  "time": "2025-11-28T09:45:14.567Z",
  "level": "INFO",
  "msg": "✅ SERP search completed successfully",
  "session_id": "abc-123-def",
  "user_id": "user-456",
  "product_count": 10,
  "relevance_score": 0.95,
  "top_products": ["iPhone 15 Pro 256GB", "iPhone 15 Pro Max", "iPhone 15 Pro 512GB"]
}
```

### SERP Error
```json
{
  "time": "2025-11-28T09:45:15.789Z",
  "level": "ERROR",
  "msg": "❌ SERP API error",
  "session_id": "abc-123-def",
  "error": "quota exceeded",
  "duration_seconds": 1.23,
  "attempt": 1,
  "max_attempts": 4,
  "key_index": 2
}
```

## Querying Logs in Grafana

### Access Loki in Grafana
1. Open Grafana: http://localhost:3001
2. Login: admin/admin
3. Go to Explore
4. Select Loki datasource

### Common Queries

See [grafana/loki-queries.md](../grafana/loki-queries.md) for detailed query examples.

#### Track User's Search Journey
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "YOUR_SESSION_ID"
```

#### Monitor SERP Performance
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP response received"
  | json
  | duration_seconds > 2
```

#### Debug Failed Searches
```logql
{compose_service="mylittleprice-backend"}
  |~ "SERP (API error|No relevant products)"
  | json
```

## Adding Logs to Code

### Basic Logging
```go
import (
    "log/slog"
    "mylittleprice/internal/utils"
)

// Info log
utils.LogInfo(ctx, "Operation completed",
    slog.String("operation", "create_user"),
    slog.Int("user_count", 42),
)

// Error log
utils.LogError(ctx, "Operation failed", err,
    slog.String("operation", "delete_user"),
)

// Warning
utils.LogWarn(ctx, "Unexpected condition",
    slog.String("reason", "user_not_found"),
)
```

### With Context
```go
// Add user_id and session_id to context
ctx = utils.WithUserID(ctx, userID.String())
ctx = utils.WithSessionID(ctx, sessionID)

// All logs from this point will include these fields
utils.LogInfo(ctx, "User action", slog.String("action", "search"))
```

## Best Practices

### ✅ DO
- Use structured fields instead of string interpolation
- Include relevant context (user_id, session_id)
- Log at appropriate levels
- Log key business events (searches, errors, performance issues)
- Use emoji prefixes for visual scanning (🔍 search, ❌ error, ✅ success)

```go
// Good
utils.LogInfo(ctx, "Search completed",
    slog.String("query", query),
    slog.Int("results", len(products)),
)
```

### ❌ DON'T
- Don't use fmt.Printf for production logs
- Don't log sensitive data (passwords, API keys, full tokens)
- Don't log at DEBUG level in production (performance impact)
- Don't use string concatenation

```go
// Bad
fmt.Printf("User %s searched for %s and found %d results\n", userID, query, len(products))
```

## Debugging Workflow

### 1. Find Your Session ID
- Check browser localStorage: `chatStore.sessionId`
- Or check WebSocket logs

### 2. Track Request Flow
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "YOUR_SESSION_ID"
  | line_format "{{.time}} [{{.level}}] {{.msg}}"
```

### 3. Identify Issues
Look for:
- ERROR level logs
- High duration_seconds (>2s)
- "No relevant products" messages
- "Quota error" messages

### 4. Analyze Performance
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP response received"
  | json
  | session_id = "YOUR_SESSION_ID"
  | line_format "{{.query}}: {{.duration_seconds}}s"
```

### 5. Check Product Results
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search completed"
  | json
  | session_id = "YOUR_SESSION_ID"
  | line_format "Found {{.product_count}} products: {{.top_products}}"
```

## Log Retention

- Logs are stored in Loki for **7 days** by default
- Older logs are automatically purged
- For long-term analysis, export to external storage

## Performance Considerations

- Structured logging has minimal performance overhead
- Loki Writer uses batching to avoid blocking
- LOG_LEVEL=info in production (debug is verbose)
- Logs are asynchronous to avoid blocking requests

## Troubleshooting

### Logs Not Appearing in Grafana
1. Check Promtail is running: `docker-compose ps promtail`
2. Check Promtail logs: `docker-compose logs promtail`
3. Verify label filters in Loki queries
4. Check backend LOG_FORMAT=json (required for structured parsing)

### Missing Context Fields
- Ensure context is properly propagated through function calls
- Use `utils.WithUserID()` and `utils.WithSessionID()` helpers
- Check that `ctx` parameter is not nil

### High Log Volume
- Increase LOG_LEVEL to warn or error
- Disable debug logs in production
- Review overly verbose log statements
