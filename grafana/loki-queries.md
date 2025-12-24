# Loki Query Examples for MyLittlePrice

## SERP API Request Tracking

### Track all SERP searches by user
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | line_format "{{.time}} [{{.user_id}}] {{.session_id}}: {{.query}} ({{.country}}, {{.search_type}})"
```

### Track SERP searches with user context
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search initiated"
  | json
  | session_id != ""
```

### Track SERP search results by user
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search completed successfully"
  | json
  | line_format "{{.time}} [{{.user_id}}] Session: {{.session_id}} - Found {{.product_count}} products (relevance: {{.relevance_score}})"
```

### Track SERP errors for specific user
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP API error"
  | json
  | user_id = "YOUR_USER_ID_HERE"
  | line_format "{{.time}} Session: {{.session_id}} - Error: {{.error}}"
```

### Track user's complete search journey
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "YOUR_SESSION_ID_HERE"
  | line_format "{{.time}} [{{.level}}] {{.msg}}"
```

### Monitor SERP API performance
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP response received"
  | json
  | duration_seconds > 2
  | line_format "Slow request: {{.query}} took {{.duration_seconds}}s (key: {{.key_index}})"
```

### Track quota errors
```logql
{compose_service="mylittleprice-backend"}
  |= "Quota error detected"
  | json
  | line_format "{{.time}} Key {{.key_index}} exhausted"
```

### Track cached vs fresh SERP requests
```logql
{compose_service="mylittleprice-backend"}
  |= "Using cached SERP results"
  | json
  | line_format "{{.time}} Cache HIT: {{.cache_key}}"
```

### Debug specific search query
```logql
{compose_service="mylittleprice-backend"}
  | json
  | query =~ ".*iphone.*"
  | line_format "{{.time}} [{{.level}}] {{.msg}} - {{.query}}"
```

## User Activity Tracking

### Track all activity for a specific user
```logql
{compose_service="mylittleprice-backend"}
  | json
  | user_id = "YOUR_USER_ID_HERE"
```

### Track anonymous users (no user_id)
```logql
{compose_service="mylittleprice-backend"}
  | json
  | user_id = ""
  | session_id != ""
```

### Track message processing
```logql
{compose_service="mylittleprice-backend"}
  |= "message processing completed"
  | json
  | line_format "{{.time}} Session: {{.session_id}} - Status: {{.status}} ({{.duration_seconds}}s)"
```

## Product Search Analysis

### Count searches by country
```logql
sum by (country) (
  count_over_time(
    {compose_service="mylittleprice-backend"}
      |= "SERP search initiated"
      | json [5m]
  )
)
```

### Average search duration
```logql
avg_over_time(
  {compose_service="mylittleprice-backend"}
    |= "SERP response received"
    | json
    | unwrap duration_seconds [5m]
)
```

### Search success rate
```logql
sum(
  rate(
    {compose_service="mylittleprice-backend"}
      |= "SERP search completed successfully" [5m]
  )
)
/
sum(
  rate(
    {compose_service="mylittleprice-backend"}
      |= "SERP search initiated" [5m]
  )
)
```

## Error Tracking

### All errors for a session
```logql
{compose_service="mylittleprice-backend"}
  | json
  | level = "ERROR"
  | session_id = "YOUR_SESSION_ID_HERE"
```

### SERP API failures
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP API error"
  | json
  | line_format "{{.time}} [{{.session_id}}] {{.error}} (attempt {{.attempt}}/{{.max_attempts}})"
```

## Real-time Monitoring

### Live tail for specific user
```logql
{compose_service="mylittleprice-backend"}
  | json
  | user_id = "YOUR_USER_ID_HERE"
```

### Live tail for all SERP requests
```logql
{compose_service="mylittleprice-backend"}
  |~ "SERP (search initiated|response received|search completed)"
```

## Tips

1. **Find your user_id**: Check the JWT token or database
2. **Find your session_id**: Check browser localStorage or WebSocket connection logs
3. **Time range**: Use the time picker in Grafana to narrow down results
4. **Live tail**: Use Grafana's "Live" mode to see real-time logs
5. **Export**: You can export query results to CSV for further analysis

## Common Debugging Scenarios

### "Why didn't my search return results?"
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "YOUR_SESSION_ID"
  |~ "SERP|relevant products|No relevant"
```

### "Why is my search slow?"
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP response received"
  | json
  | session_id = "YOUR_SESSION_ID"
  | line_format "Duration: {{.duration_seconds}}s with key {{.key_index}}"
```

### "Which products were returned?"
```logql
{compose_service="mylittleprice-backend"}
  |= "SERP search completed successfully"
  | json
  | session_id = "YOUR_SESSION_ID"
  | line_format "Products: {{.top_products}}"
```
