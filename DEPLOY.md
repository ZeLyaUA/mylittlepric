# Deployment Guide for Dokploy

This guide explains how to deploy MyLittlePrice services using Dokploy.

## Prerequisites

- Dokploy instance running
- Git repository access
- PostgreSQL and Redis instances (can be deployed separately in Dokploy)

## Services to Deploy

You need to deploy 4 separate services in Dokploy:

1. **PostgreSQL Database**
2. **Redis Cache**
3. **Backend API**
4. **Frontend**

---

## 1. PostgreSQL Database

### Service Configuration
- **Type:** Database
- **Database Type:** PostgreSQL 17
- **Database Name:** `mylittleprice`
- **Username:** `postgres`
- **Password:** *(set your secure password)*
- **Port:** `5432`

### Volumes
- Data: `/var/lib/postgresql/data`

### Notes
- After deployment, note the internal connection string for the backend
- Connection string format: `postgres://username:password@postgres-service:5432/mylittleprice?sslmode=disable`

---

## 2. Redis Cache

### Service Configuration
- **Type:** Database
- **Database Type:** Redis 8
- **Port:** `6379`

### Command Override
```bash
redis-server --appendonly yes
```

### Volumes
- Data: `/data`

### Notes
- Note the internal connection string for the backend
- Connection string format: `redis-service:6379`

---

## 3. Backend API

### Service Configuration
- **Type:** Application
- **Source:** Git Repository
- **Repository:** *(your repository URL)*
- **Branch:** `main`
- **Build Type:** Dockerfile
- **Dockerfile Path:** `backend/Dockerfile`
- **Context Path:** `backend`
- **Port:** `8080`

### Environment Variables

**Required:**
```bash
# Database
DATABASE_URL=postgres://postgres:YOUR_PASSWORD@postgres-service:5432/mylittleprice?sslmode=disable

# Redis
REDIS_URL=redis-service:6379

# JWT Secrets (generate with: openssl rand -hex 32)
JWT_ACCESS_SECRET=your_access_secret_here
JWT_REFRESH_SECRET=your_refresh_secret_here
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=7d

# Google OAuth
GOOGLE_CLIENT_ID=your_google_client_id
GOOGLE_CLIENT_SECRET=your_google_client_secret
GOOGLE_REDIRECT_URL=https://your-frontend-domain.com

# API Keys (comma-separated for rotation)
GEMINI_API_KEYS=key1,key2,key3
SERP_API_KEYS=key1,key2,key3

# CORS
CORS_ORIGINS=https://your-frontend-domain.com,http://localhost:3000

# Server
PORT=8080
SERVER_ID=backend-1
LOG_LEVEL=info

# Session
DEFAULT_SESSION_TTL=24h
```

### Health Check
- **Endpoint:** `/health`
- **Port:** `8080`
- **Interval:** `30s`

### Domain
- Configure your backend domain (e.g., `api.yourdomain.com`)
- Enable HTTPS/SSL

### Notes
- Make sure PostgreSQL and Redis services are running before starting the backend
- The backend will automatically run migrations on startup

---

## 4. Frontend

### Service Configuration
- **Type:** Application
- **Source:** Git Repository
- **Repository:** *(your repository URL)*
- **Branch:** `main`
- **Build Type:** Dockerfile
- **Dockerfile Path:** `frontend/Dockerfile`
- **Context Path:** `frontend`
- **Port:** `3000`

### Build Arguments (ARG)
These are needed during the build process:
```bash
NEXT_PUBLIC_API_URL=https://api.yourdomain.com
NEXT_PUBLIC_GOOGLE_CLIENT_ID=your_google_client_id
NEXT_PUBLIC_SITE_URL=https://yourdomain.com
```

### Environment Variables (Runtime)
```bash
NODE_ENV=production
PORT=3000
HOSTNAME=0.0.0.0
```

### Health Check
- **Port:** `3000`
- **Interval:** `30s`

### Domain
- Configure your frontend domain (e.g., `yourdomain.com`)
- Enable HTTPS/SSL

### Notes
- The frontend connects to the backend via `NEXT_PUBLIC_API_URL`
- Make sure the backend is running before starting the frontend
- Update CORS_ORIGINS in backend to include your frontend domain

---

## Deployment Order

1. **Deploy PostgreSQL** → Wait for healthy status
2. **Deploy Redis** → Wait for healthy status
3. **Deploy Backend** → Wait for healthy status and check logs
4. **Deploy Frontend** → Wait for healthy status

---

## Post-Deployment Verification

### Backend Health Check
```bash
curl https://api.yourdomain.com/health
```
Expected response:
```json
{
  "status": "ok",
  "timestamp": "2025-01-01T00:00:00Z"
}
```

### Frontend Health Check
```bash
curl https://yourdomain.com
```
Should return the HTML of your landing page.

### Test WebSocket Connection
1. Open your frontend in a browser
2. Navigate to `/chat`
3. Send a test message
4. Check browser console for WebSocket connection status
5. Check backend logs for WebSocket activity

---

## Monitoring (Optional)

If you want to deploy the monitoring stack:

### Prometheus
- **Type:** Application
- **Image:** `prom/prometheus:latest`
- **Port:** `9090`
- **Config:** Mount `prometheus/prometheus.yml` as volume

### Grafana
- **Type:** Application
- **Image:** `grafana/grafana:latest`
- **Port:** `3001`
- **Volumes:** Mount `grafana/` directory

### Loki
- **Type:** Application
- **Image:** `grafana/loki:latest`
- **Port:** `3100`
- **Config:** Mount `loki/loki-config.yml`

---

## Troubleshooting

### Backend won't start
1. Check PostgreSQL connection:
   ```bash
   # In Dokploy terminal for backend service
   wget -O- postgres-service:5432
   ```
2. Check Redis connection:
   ```bash
   wget -O- redis-service:6379
   ```
3. Verify environment variables are set correctly
4. Check logs for specific error messages

### Frontend won't connect to backend
1. Verify `NEXT_PUBLIC_API_URL` is set correctly during build
2. Check CORS settings in backend
3. Verify backend is accessible from frontend container
4. Check browser console for CORS errors

### Database migrations not running
1. Check backend logs for migration errors
2. Verify `migrations/` directory is included in Docker image
3. Manually run migrations if needed:
   ```bash
   # Connect to backend container
   # Run migrations manually
   ```

### WebSocket connection issues
1. Check that WebSocket endpoint is accessible: `wss://api.yourdomain.com/ws`
2. Verify CORS origins include your frontend domain
3. Check browser console for WebSocket errors
4. Ensure Dokploy/proxy supports WebSocket upgrades

---

## Security Checklist

- [ ] Change default PostgreSQL password
- [ ] Generate secure JWT secrets (use `openssl rand -hex 32`)
- [ ] Configure CORS origins to only allow your frontend domain
- [ ] Enable HTTPS/SSL for both frontend and backend
- [ ] Set `LOG_LEVEL=info` or `warn` in production (not `debug`)
- [ ] Rotate API keys regularly (Gemini, SERP)
- [ ] Use environment variables, never commit secrets to Git
- [ ] Enable rate limiting in backend
- [ ] Configure firewall rules to restrict database access

---

## Scaling Considerations

### Horizontal Scaling (Multiple Backend Instances)
The backend is designed to scale horizontally:
- Redis Pub/Sub enables WebSocket message broadcasting across instances
- Session state stored in Redis (shared across instances)
- Stateless design (no in-memory state)

To scale:
1. Deploy multiple backend instances in Dokploy
2. Use a load balancer (Dokploy proxy handles this automatically)
3. Ensure all instances use the same Redis instance
4. Set unique `SERVER_ID` for each instance for debugging

### Database Scaling
- Consider read replicas for PostgreSQL if needed
- Enable connection pooling (already configured in Ent ORM)
- Monitor query performance with Prometheus metrics

### Redis Scaling
- For high traffic, consider Redis Cluster
- Enable persistence (AOF) for data durability
- Monitor memory usage

---

## Backup Strategy

### PostgreSQL Backups
```bash
# Create backup
docker exec postgres-service pg_dump -U postgres mylittleprice > backup.sql

# Restore backup
docker exec -i postgres-service psql -U postgres mylittleprice < backup.sql
```

### Redis Backups
Redis with AOF enabled automatically persists data to `/data/appendonly.aof`

---

## Support

For issues specific to:
- **Dokploy:** Check Dokploy documentation
- **Application:** Create an issue in the repository
- **Database/Redis:** Check respective official documentation
