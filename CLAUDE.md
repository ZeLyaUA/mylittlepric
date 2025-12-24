# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MyLittlePrice is a full-stack AI-powered shopping assistant that leverages Google Gemini AI for intelligent product recommendations and web search. The application features real-time WebSocket communication, multi-user authentication (email + Google OAuth), and comprehensive monitoring infrastructure.

**Tech Stack:**
- **Backend:** Go 1.24 with Fiber web framework
- **Frontend:** Next.js 16 (App Router) with React 19 & TypeScript
- **Database:** PostgreSQL 17 with Ent ORM
- **Cache/PubSub:** Redis 8
- **AI:** Google Gemini API
- **Search:** SerpAPI (Google Shopping Search)
- **Monitoring:** Prometheus, Loki, Grafana, AlertManager

## Development Commands

### Backend (Go)

```bash
# Navigate to backend directory
cd backend

# Run development server
go run ./cmd/api/main.go

# Build binary
go build -o api ./cmd/api/

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/services/...

# Generate Ent ORM code after schema changes
go generate ./ent

# Install dependencies
go mod download

# Tidy dependencies
go mod tidy
```

### Frontend (Next.js)

```bash
# Navigate to frontend directory
cd frontend

# Run development server with Turbopack
npm run dev

# Build for production
npm run build

# Start production server
npm start

# Install dependencies
npm install
```

### Infrastructure

```bash
# Start PostgreSQL + Redis
docker-compose up

# Start in detached mode
docker-compose up -d

# Stop services
docker-compose down

# Start monitoring stack (Prometheus, Grafana, Loki, AlertManager)
docker-compose -f docker-compose.monitoring.yml up

# View logs
docker-compose logs -f
```

### Typical Development Workflow

```bash
# Terminal 1: Start infrastructure
docker-compose up

# Terminal 2: Start backend
cd backend && go run ./cmd/api/main.go

# Terminal 3: Start frontend
cd frontend && npm run dev

# Terminal 4 (Optional): Start monitoring
docker-compose -f docker-compose.monitoring.yml up
```

**Development URLs:**
- Frontend: http://localhost:3000
- Backend API: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3001 (admin/admin)
- PostgreSQL: localhost:5432
- Redis: localhost:6379

## Architecture

### High-Level Structure

```
mylittleprice/
├── backend/           # Go backend service
│   ├── cmd/api/       # Application entry point (main.go)
│   ├── internal/      # Private application code
│   ├── ent/           # Ent ORM generated code & schemas
│   └── migrations/    # Database migration SQL files
├── frontend/          # Next.js frontend application
│   └── src/
│       ├── app/       # App Router routes
│       ├── features/  # Feature-based components
│       └── shared/    # Shared utilities & components
├── prometheus/        # Prometheus configuration
├── grafana/           # Grafana dashboards & provisioning
├── loki/              # Loki log aggregation config
├── promtail/          # Log shipper config
└── alertmanager/      # Alert routing config
```

### Backend Architecture

**Entry Point:** [backend/cmd/api/main.go](backend/cmd/api/main.go)

The backend follows a **dependency injection container pattern** where all services are initialized in [backend/internal/container/container.go](backend/internal/container/container.go) and passed to handlers.

**Key Layers:**
1. **Handlers** ([internal/handlers/](backend/internal/handlers/)) - HTTP & WebSocket request handling
2. **Services** ([internal/services/](backend/internal/services/)) - Business logic (Gemini, SERP, Auth, Session, etc.)
3. **Ent ORM** ([ent/schema/](backend/ent/schema/)) - Database models and queries
4. **Middleware** ([internal/middleware/](backend/internal/middleware/)) - Auth, CORS, rate limiting, Prometheus metrics

**Routing:** All routes are defined in [backend/internal/app/routes.go](backend/internal/app/routes.go)

**WebSocket Architecture:**
- Real-time chat communication via `/ws` endpoint
- Redis Pub/Sub for cross-server message broadcasting (horizontal scaling ready)
- Connection pooling per user with server ID tracking
- Message types: `chat`, `response`, `products`, `quick_replies`, `sync`, `connection_ack`

**Key Services:**
- **GeminiService** - AI response generation, grounding decisions
- **SerpService** - Google Shopping search integration
- **SessionService** - Session lifecycle management with 24h expiration
- **MessageService** - Chat message persistence
- **AuthService** - Email + Google OAuth authentication with JWT
- **CacheService** - Redis caching layer
- **EmbeddingService** - Text embeddings for context analysis
- **CleanupService** - Background job for expired session cleanup

### Frontend Architecture

**Framework:** Next.js 16 App Router with route groups

**Route Groups:**
- `(marketing)` - Public pages: landing, policies (/, /privacy-policy, /terms-of-use)
- `(auth)` - Authentication: /login
- `(app)` - Protected routes with auth guard: /chat, /history, /settings

**State Management:**
- **Zustand** stores with localStorage persistence
- **auth-store.ts** - User authentication state (tokens, user info)
- **store.ts** - Chat state (messages, session, preferences, rate limits)

**Key Hooks:**
- `useChat()` - WebSocket connection, message sending, rate limit tracking
- `useApi()` - REST API calls with authentication headers
- `useLocalStorage()` - Persistent local storage

**Feature Modules:** ([frontend/src/features/](frontend/src/features/))
- **auth** - AuthDialog, UserMenu, OAuth integration
- **chat** - ChatInterface, ChatMessages, ChatInput (main UI)
- **products** - ProductCard, ProductDrawer, product display components
- **search** - SearchHistory, SearchInput

### Database Schema (Ent ORM)

**Schema Location:** [backend/ent/schema/](backend/ent/schema/)

**Entities:**
- **User** - Email/Google OAuth users, password hashes, provider info
- **ChatSession** - Conversation sessions with JSONB state (search state, cycle state, conversation context), 24h default TTL
- **Message** - Chat messages (role: user/assistant, content)
- **SearchHistory** - Search queries with product results (JSONB)
- **UserPreference** - Locale, currency, country preferences

**Migrations:** Numbered SQL files in [backend/migrations/](backend/migrations/) (001-010)

**Indexes:**
- User: email (unique)
- ChatSession: (user_id, expires_at), (expires_at), (session_id)
- SearchHistory: (user_id, created_at)

**After Schema Changes:**
1. Modify schema in `backend/ent/schema/`
2. Run `go generate ./ent` to regenerate code
3. Create a new migration SQL file in `backend/migrations/`
4. Test migration locally before deploying

## Configuration

### Backend Environment Variables

**File:** [backend/.env.example](backend/.env.example)

**Critical Settings:**
- `DATABASE_URL` - PostgreSQL connection string
- `REDIS_URL` - Redis connection (host:port)
- `JWT_ACCESS_SECRET` / `JWT_REFRESH_SECRET` - Generate with `openssl rand -hex 32`
- `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` - OAuth credentials from Google Cloud Console
- `GEMINI_API_KEYS` - Comma-separated Gemini API keys (supports rotation)
- `SERP_API_KEYS` - Comma-separated SerpAPI keys (supports rotation)
- `CORS_ORIGINS` - Comma-separated allowed origins for CORS

**Key Rotation:** The backend supports multiple API keys for both Gemini and SERP with automatic rotation on quota errors.

### Frontend Environment Variables

**Required:**
- `NEXT_PUBLIC_API_URL` - Backend API URL (e.g., http://localhost:8080)
- `NEXT_PUBLIC_GOOGLE_CLIENT_ID` - Google OAuth client ID
- `NEXT_PUBLIC_SITE_URL` - Frontend URL (e.g., http://localhost:3000)

## Common Development Tasks

### Adding a New API Endpoint

1. Create handler function in [backend/internal/handlers/](backend/internal/handlers/)
2. Add business logic in [backend/internal/services/](backend/internal/services/)
3. Register route in [backend/internal/app/routes.go](backend/internal/app/routes.go)
4. Add middleware (auth, rate limit) as needed
5. Test endpoint

### Adding a New Database Entity

1. Create schema file in [backend/ent/schema/](backend/ent/schema/)
2. Run `go generate ./ent` to generate ORM code
3. Create migration SQL file in [backend/migrations/](backend/migrations/)
4. Add service methods in [backend/internal/services/](backend/internal/services/)
5. Create handlers to expose via API

### Adding a New Frontend Page

1. Create route folder in [frontend/src/app/](frontend/src/app/)
2. Add `page.tsx` component
3. Create feature components in [frontend/src/features/](frontend/src/features/)
4. Use `useApi()` hook for backend calls
5. Update Zustand store if global state is needed ([frontend/src/shared/lib/store.ts](frontend/src/shared/lib/store.ts))

### Working with WebSocket Messages

**Backend:** [backend/internal/handlers/websocket.go](backend/internal/handlers/websocket.go), [backend/internal/handlers/processor.go](backend/internal/handlers/processor.go)

**Frontend:** [frontend/src/features/chat/hooks/index.ts](frontend/src/features/chat/hooks/index.ts)

**Message Flow:**
1. User sends message via `useChat().sendMessage()`
2. WebSocket sends to `/ws` endpoint
3. Backend validates session ownership
4. `ChatProcessor.ProcessChat()` handles message
5. Gemini generates response (with optional grounding)
6. SERP searches products if needed
7. Response broadcast via Redis Pub/Sub
8. Frontend receives and displays in ChatMessages component

## Monitoring & Observability

### Prometheus Metrics

**Endpoint:** http://localhost:8080/metrics

**Metrics Collected:**
- HTTP request duration & count (by path, method, status)
- WebSocket connections & messages
- Gemini API usage (tokens, grounding decisions)
- API key rotation events
- Session lifecycle events

### Logs (Loki)

**Architecture:** Application (slog) → Promtail → Loki → Grafana

**Structured Logging:** Uses Go's `slog` package with JSON format

**Log Levels:** Configured via `LOG_LEVEL` environment variable (debug, info, warn, error)

**Automatic Context Fields:**
- `session_id`: Automatically included in all session-related logs
- `user_id`: Automatically included for authenticated users
- `request_id`: Unique identifier for each request

**SERP API Logging:**
All SERP searches are comprehensively logged with:
- Search parameters (query, country, search_type, price range)
- API key usage (key_index)
- Performance metrics (duration_seconds)
- Results (product_count, relevance_score, top_products)
- Errors and retry attempts

**Quick Start:**
- View logs in Grafana: http://localhost:3001 → Explore → Loki
- See query examples: [grafana/loki-queries.md](grafana/loki-queries.md)
- Detailed guide: [docs/LOGGING.md](docs/LOGGING.md) (English)
- Quick guide: [docs/LOGGING_RU.md](docs/LOGGING_RU.md) (Russian)

**Example Query - Track User's Search:**
```logql
{compose_service="mylittleprice-backend"}
  | json
  | session_id = "YOUR_SESSION_ID"
  |~ "SERP"
```

### Grafana Dashboards

**URL:** http://localhost:3001 (admin/admin)

**Features:**
- Pre-provisioned datasources (Prometheus, Loki)
- Dashboard auto-loading from [grafana/dashboards/](grafana/dashboards/)
- Query builder for custom metrics
- Live log streaming for real-time debugging

## Important Patterns

### Dependency Injection Container

All services are initialized once in [backend/internal/container/container.go](backend/internal/container/container.go) and passed to handlers. This pattern:
- Simplifies testing (easy to mock services)
- Ensures single database/Redis connection pool
- Centralizes configuration
- Makes dependencies explicit

**Handler Pattern:**
```go
func NewMyHandler(container *container.Container) fiber.Handler {
    return func(c *fiber.Ctx) error {
        // Access services via container.MyService
    }
}
```

### Service Layer Pattern

Business logic lives in services, not handlers. Handlers are thin wrappers that:
1. Parse/validate input
2. Call service methods
3. Format response
4. Handle errors

**Example:** Authentication flow uses `AuthService.SignUp()` → `JWTService.GenerateTokens()` → return to handler

### Rate Limiting

**HTTP Routes:** Redis-backed sliding window ([backend/internal/middleware/rate_limiter.go](backend/internal/middleware/rate_limiter.go))

**WebSocket:** Per-connection token bucket with rate limit headers sent to client

**Frontend Tracking:** Rate limit state tracked in Zustand store and displayed to user

### State Management (Frontend)

**Zustand Pattern:**
- Store definition in [frontend/src/shared/lib/](frontend/src/shared/lib/)
- Persist to localStorage for auth state
- Sync preferences bidirectionally with backend
- Actions as methods on store

### Authentication Flow

1. **Email Auth:** POST `/api/auth/signup` or `/api/auth/login` → JWT tokens
2. **Google OAuth:** Google popup → POST `/api/auth/google` with OAuth token → JWT tokens
3. **Token Storage:** Zustand + localStorage
4. **Token Refresh:** Background interval checks expiry → POST `/api/auth/refresh`
5. **Protected Routes:** Auth middleware validates JWT on API routes, layout guards frontend routes

## Security Considerations

- JWT secrets must be randomly generated in production (use `openssl rand -hex 32`)
- Passwords are hashed with bcrypt (cost factor 10)
- CORS origins must be explicitly configured via `CORS_ORIGINS` environment variable
- Rate limiting prevents abuse on auth and WebSocket endpoints
- Session ownership validated before any session operation
- All user input sanitized before passing to Gemini/SERP APIs

## Testing

**Backend:**
```bash
# Run all tests
go test ./...

# Run specific package
go test ./internal/services/

# With coverage
go test -cover ./...

# Verbose output
go test -v ./...
```

**Frontend:**
No test files currently present in src/ directory. Consider adding:
- Unit tests with Jest/Vitest
- Component tests with React Testing Library
- E2E tests with Playwright

## Performance Considerations

- **Caching:** Redis caches session context, preferences, and search results
- **API Key Rotation:** Automatic failover on quota errors reduces downtime
- **WebSocket Scaling:** Redis Pub/Sub enables horizontal scaling across multiple backend instances
- **Database Indexes:** Critical queries indexed (see schema files)
- **Session Cleanup:** Background job (`CleanupService`) removes expired sessions to prevent table bloat

## Key Files Reference

**Must-Read for New Contributors:**
- [backend/cmd/api/main.go](backend/cmd/api/main.go) - Server initialization
- [backend/internal/app/routes.go](backend/internal/app/routes.go) - All API routes
- [backend/internal/container/container.go](backend/internal/container/container.go) - Dependency setup
- [backend/internal/handlers/processor.go](backend/internal/handlers/processor.go) - Chat message processing
- [backend/internal/services/gemini.go](backend/internal/services/gemini.go) - AI integration
- [frontend/src/shared/lib/store.ts](frontend/src/shared/lib/store.ts) - Global state
- [frontend/src/features/chat/hooks/index.ts](frontend/src/features/chat/hooks/index.ts) - WebSocket hook
- [frontend/src/app/(app)/layout.tsx](frontend/src/app/(app)/layout.tsx) - Auth guard
- [frontend/src/features/chat/components/ChatInterface.tsx](frontend/src/features/chat/components/ChatInterface.tsx) - Main UI

## Troubleshooting

**Database connection errors:**
- Verify PostgreSQL is running: `docker-compose ps`
- Check `DATABASE_URL` in `.env`
- Ensure migrations ran: Check Docker logs

**Redis connection errors:**
- Verify Redis is running: `docker-compose ps`
- Check `REDIS_URL` in `.env`

**WebSocket not connecting:**
- Check CORS origins configuration
- Verify auth token is valid
- Check browser console for errors

**Gemini/SERP API errors:**
- Verify API keys are valid
- Check quota limits
- Review logs for specific error messages

**Rate limit errors:**
- Check Redis connection (rate limiter stores state in Redis)
- Review rate limit configuration in `.env`
- Clear rate limit: Restart Redis or wait for TTL expiration
