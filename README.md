# Check-in Service - Usage Guide

## Quick Start

### 1. Start the System

```bash
# Clone and enter directory
cd checkin-service

# Create go.mod
go mod init checkin-service
go mod tidy

# Start all services with Docker Compose
docker-compose up -d

# Check if services are running
docker-compose ps
```

### 2. Verify Services

**RabbitMQ Management UI:**
- URL: http://localhost:15672
- Username: `guest`
- Password: `guest`
- Check exchanges and queues are created

**MailHog (Email UI):**
- URL: http://localhost:8025
- View sent emails here

**Service Health:**
```bash
curl http://localhost:8080/health
# Should return: {"status":"healthy"}
```

---

## Testing the API

### Check-In Flow

```bash
# Employee EMP001 checks in
curl -X POST http://localhost:8080/api/checkin \
  -H "Content-Type: application/json" \
  -d '{"employee_id": "EMP001"}'

# Response:
# {
#   "success": true,
#   "message": "Successfully checked in",
#   "record_id": "uuid-here",
#   "action": "checked_in"
# }
```

### Check-Out Flow

```bash
# Same employee checks out (same endpoint)
curl -X POST http://localhost:8080/api/checkin \
  -H "Content-Type: application/json" \
  -d '{"employee_id": "EMP001"}'

# Response:
# {
#   "success": true,
#   "message": "Successfully checked out",
#   "record_id": "uuid-here",
#   "action": "checked_out",
#   "hours_worked": 8.5
# }
```

### What Happens on Check-Out?

1. ✅ Time record saved to database
2. ✅ Event published to RabbitMQ exchange `checkout-events`
3. ✅ Event routed to 2 queues:
   - `labor-cost-queue` → Labor Cost Worker processes
   - `email-queue` → Email Worker processes
4. ✅ Labor Cost Worker retries if legacy API fails
5. ✅ Email sent (visible in MailHog UI)

---

## Monitoring

### View RabbitMQ Queues

Go to http://localhost:15672 and check:
- **Exchange:** `checkout-events` (fanout type)
- **Queues:**
  - `labor-cost-queue` (with DLQ: `labor-cost-queue-dlq`)
  - `email-queue` (with DLQ: `email-queue-dlq`)

### Check Messages

```bash
# View messages in labor-cost-queue
# Use RabbitMQ Management UI > Queues > labor-cost-queue > Get Messages
```

### View Logs

```bash
# All services
docker-compose logs -f

# Just the check-in service
docker-compose logs -f checkin-service

# Just RabbitMQ
docker-compose logs -f rabbitmq
```

### Database Queries

```bash
# Connect to database
docker exec -it <postgres-container-id> psql -U checkin_user -d checkin_db

# View time records
SELECT * FROM time_records;

# View active check-ins
SELECT * FROM time_records WHERE status = 'CHECKED_IN';

# View completed shifts
SELECT employee_id, check_in_at, check_out_at, hours_worked 
FROM time_records 
WHERE status = 'CHECKED_OUT'
ORDER BY check_out_at DESC;
```

---

## Testing Failure Scenarios

### 1. Legacy API Down (Retry Logic)

```bash
# Stop the mock legacy API
docker-compose stop legacy-api-mock

# Check someone out
curl -X POST http://localhost:8080/api/checkin \
  -H "Content-Type: application/json" \
  -d '{"employee_id": "EMP002"}'

# Check logs - you'll see retry attempts
docker-compose logs -f checkin-service
# Output shows: "Retry 1/5 for employee EMP002..."

# Restart legacy API - message will be processed
docker-compose start legacy-api-mock
```

### 2. Dead Letter Queue

After max retries (5 attempts), failed messages go to DLQ.

View in RabbitMQ UI:
- Queue: `labor-cost-queue-dlq`
- See failed messages with headers showing retry count

### 3. Email Service Down

```bash
# Stop MailHog
docker-compose stop mailhog

# Check someone out - checkout still succeeds
curl -X POST http://localhost:8080/api/checkin \
  -H "Content-Type: application/json" \
  -d '{"employee_id": "EMP003"}'

# Message queued in email-queue
# Restart MailHog - email will be sent
docker-compose start mailhog
```

---

## Load Testing

```bash
# Install hey (HTTP load testing tool)
go install github.com/rakyll/hey@latest

# Simulate 100 check-ins
for i in {1..100}; do
  curl -X POST http://localhost:8080/api/checkin \
    -H "Content-Type: application/json" \
    -d "{\"employee_id\": \"EMP$(printf "%03d" $i)\"}" &
done

# Check queue depth in RabbitMQ UI
# Messages should be processed by workers
```

---

## Project Structure

```
checkin-service/
├── cmd/
│   └── api/
│       └── main.go                 # Application entry point
├── domain/
│   ├── entities/
│   │   └── time_record.go         # Core business entity
│   ├── events/
│   │   └── domain_events.go       # Domain events (with versioning)
│   └── repositories/
│       └── time_record_repository.go # Repository interface
├── application/
│   ├── services/
│   │   ├── checkin_service.go     # Check-in use case
│   │   └── checkout_service.go    # Check-out use case
│   └── handlers/
│       ├── labor_cost_handler.go  # Event handler for labor cost
│       └── email_handler.go       # Event handler for email
├── infrastructure/
│   ├── config/
│   │   ├── env.go                 # Centralized config loader
│   │   ├── logger.go              # Zap logger setup
│   │   └── otel.go                # OpenTelemetry setup
│   ├── persistence/
│   │   └── postgres_repository.go # Database implementation
│   ├── messaging/
│   │   ├── rabbitmq_publisher.go  # Event publisher
│   │   └── rabbitmq_consumer.go   # Event consumer
│   └── external/
│       ├── legacy_api_client.go   # Legacy API client
│       └── email_client.go        # Email client
├── presentation/
│   └── http/
│       └── handlers.go            # HTTP handlers
├── architecture.drawio            # System architecture diagram
├── Design_explanation.md          # Written architecture/design explanation
└── ...
```

---

## Key Design Patterns Used

### 1. **Domain-Driven Design (DDD)**
- **Domain Layer:** Pure business logic (TimeRecord entity)
- **Application Layer:** Use cases (CheckInService, CheckOutService)
- **Infrastructure Layer:** Technical implementations (DB, RabbitMQ)
- **Presentation Layer:** HTTP API

### 2. **Event-Driven Architecture**
- Events published after state changes
- Async processing decouples check-in/out from side effects
- Multiple consumers process same event independently

### 3. **Retry Pattern**
- Exponential backoff (1s, 2s, 4s, 8s, 16s)
- Max 5 attempts before DLQ
- Circuit breaker could be added

### 4. **Dead Letter Queue**
- Failed messages after retries → DLQ
- Manual intervention or scheduled retry
- Prevents poison messages from blocking queue

### 5. **Clean Architecture**
- Dependencies point inward (domain has no external deps)
- Interfaces define contracts
- Easy to test and swap implementations

---

## Production Considerations

### 1. **Add Health Checks**

```go
func (h *CheckInHandler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
    // Check DB connection
    if err := h.db.Ping(); err != nil {
        http.Error(w, "database unhealthy", http.StatusServiceUnavailable)
        return
    }
    
    // Check RabbitMQ connection
    if !h.publisher.IsConnected() {
        http.Error(w, "message queue unhealthy", http.StatusServiceUnavailable)
        return
    }
    
    w.WriteHeader(http.StatusOK)
}
```

### 2. **Add Metrics**

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    checkInsTotal = prometheus.NewCounter(...)
    checkOutsTotal = prometheus.NewCounter(...)
    eventProcessingDuration = prometheus.NewHistogram(...)
)
```

### 3. **Add Distributed Tracing**

```go
import "go.opentelemetry.io/otel"

ctx, span := tracer.Start(ctx, "CheckOut")
defer span.End()
```

### 4. **Environment-Based Configuration**

Use [viper](https://github.com/spf13/viper) or similar for config management.

### 5. **Graceful Shutdown**

Already implemented - catches SIGINT/SIGTERM and drains connections.

---

## Troubleshooting

### Problem: Messages not being consumed

**Solution:**
1. Check RabbitMQ UI - are messages in queues?
2. Check worker logs - are consumers running?
3. Verify queue bindings to exchange

### Problem: Database connection errors

**Solution:**
1. Verify DATABASE_URL is correct
2. Check PostgreSQL is running: `docker-compose ps`
3. Check connection pool settings

### Problem: Events lost

**Solution:**
- Events are durable and persistent
- Check `outbox_events` table (if implemented)
- Check RabbitMQ message persistence settings

---

## Stopping the System

```bash
# Stop all services
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v
```