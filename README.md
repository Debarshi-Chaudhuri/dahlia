# Dahlia Alerts - Real-Time Alerts & Escalation Engine

A minimal production-grade POC for an event-driven workflow automation system built with **Golang**, **Uber FX**, and **Gin**.

## 🚀 Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.21+
- VS Code (optional, but recommended)

### 1. Start Infrastructure
```bash
# Start all infrastructure services (LocalStack, DynamoDB, SQS, Redis, ZooKeeper)
docker-compose -f docker-compose.infra.yml up -d

# Initialize DynamoDB tables and SQS queues
./scripts/dahlia-tables.sh
./scripts/dahlia-queues.sh
```

### 2. Build Services
```bash
# Build both services
go build -o injestion ./cmd/injestion
go build -o scheduler ./cmd/scheduler
```

### 3. Run Services
```bash
# Terminal 1 - Ingestion Service (Port 8090)
./injestion

# Terminal 2 - Scheduler Service (Port 8091)  
./scheduler
```

### 4. Test Health Endpoints
```bash
# Test ingestion service
curl http://localhost:8090/health

# Test scheduler service
curl http://localhost:8091/health
```

## 🔧 VS Code Development

If using VS Code, you can use the provided configurations:

1. **Press F5** - Launch both services in debug mode
2. **Ctrl+Shift+P** → "Tasks: Run Task" → "Start Infrastructure"
3. **Ctrl+Shift+P** → "Tasks: Run Task" → "Build All Services"

Available debug configurations:
- Launch Ingestion Service
- Launch Scheduler Service  
- Launch Both Services (compound)

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Ingestion Service                       │
│                     (Port 8090)                          │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────────┐      ┌────────────────────┐      │
│  │  REST API        │      │  Workflow Manager  │      │
│  │  - POST /signals │◄────►│  - In-memory cache │      │
│  │  - POST /workflows│      │  - ZK watcher      │      │
│  │  - GET /runs     │      └────────────────────┘      │
│  └────────┬─────────┘                                   │
│           │                                              │
│           ▼                                              │
│  ┌────────────────────────────────────────────┐        │
│  │   Workflow Executor Workers (5 goroutines) │        │
│  │   - SQS consumer                           │        │
│  │   - Evaluate conditions                    │        │
│  │   - Execute actions                        │        │
│  └────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
        ┌────────────────────────────────┐
        │         AWS SQS                │
        │  - executor-queue              │
        └────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                  Scheduler Service                       │
│                     (Port 8091)                          │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────────┐      ┌────────────────────┐      │
│  │  Cron Job        │      │  Redis Buckets     │      │
│  │  (every minute)  │─────►│  [YYYY:MM:DD:HH:MM]│      │
│  └──────────────────┘      └────────────────────┘      │
│  ┌──────────────────┐                                   │
│  │  POST /schedule  │                                   │
│  └──────────────────┘                                   │
└─────────────────────────────────────────────────────────┘
```

## 🛠️ Tech Stack

### Core
- **Language:** Golang 1.21+
- **DI Framework:** Uber FX
- **Web Framework:** Gin
- **Logging:** Zap (structured logging)

### Infrastructure (Docker)
- **Database:** DynamoDB (via LocalStack)
- **Message Queue:** SQS (via LocalStack)  
- **Coordination:** ZooKeeper 3.8
- **Cache/Scheduling:** Redis 7.2

### Admin UIs
- **SQS Admin:** http://localhost:9080
- **ZooKeeper Navigator:** http://localhost:8000
- **DynamoDB:** http://localhost:9000

## 📁 Project Structure

```
dahlia/
├── cmd/
│   ├── injestion/          # Ingestion service entry point
│   │   └── main.go
│   └── scheduler/          # Scheduler service entry point
│       └── main.go
│
├── commons/                # Shared components
│   ├── config/             # Provider functions
│   ├── handler/            # Generic handlers & middleware
│   ├── response/           # Standardized response types
│   ├── routes/             # Generic route registration
│   └── server/             # HTTP server lifecycle
│
├── internal/               # Internal packages
│   ├── config/             # Service-specific providers
│   ├── handler/            # Business logic handlers
│   ├── logger/             # Logger interface & implementation
│   └── routes/             # Service-specific routes
│
├── scripts/                # Setup scripts
│   ├── dahlia-tables.sh    # Create DynamoDB tables
│   └── dahlia-queues.sh    # Create SQS queues
│
├── .vscode/                # VS Code configuration
│   ├── launch.json         # Debug configurations
│   ├── tasks.json          # Build & dev tasks
│   └── settings.json       # Editor settings
│
├── docker-compose.infra.yml # Infrastructure services
```

## 🔌 API Endpoints

### Ingestion Service (Port 8090)

#### Health Check
```bash
GET /health
```

#### Signal Ingestion
```bash
POST /api/v1/signals
Content-Type: application/json

{
  "signal_type": "internet_signal",
  "org_id": "org_123", 
  "value": {"status": 1},
  "timestamp": "2025-01-21T10:00:00Z"
}
```

#### Workflow Management
```bash
POST /api/v1/workflows
Content-Type: application/json

{
  "name": "Internet Downtime Escalation",
  "signal_type": "internet_signal",
  "conditions": [
    {
      "type": "absence",
      "duration": "5m"
    }
  ],
  "actions": [
    {
      "type": "slack", 
      "target": "#network-ops",
      "message": "No signal from {{org_id}} for 5 minutes"
    },
    {
      "type": "delay",
      "duration": "15m"
    },
    {
      "type": "slack",
      "target": "@infra-head", 
      "message": "ESCALATION: {{org_id}} still down"
    }
  ]
}
```

### Scheduler Service (Port 8091)

#### Health Check
```bash
GET /health
```

#### Job Scheduling
```bash
POST /api/v1/schedule
Content-Type: application/json

{
  "run_id": "uuid",
  "delay_seconds": 900,
  "job_details": {
    "signal_id": "uuid",
    "workflow_id": "uuid", 
    "resume_from": "ACTION_2"
  }
}
```

## 🧪 Development

### Running Tests
```bash
# Unit tests
go test ./...

# Integration tests (requires infrastructure)
docker-compose -f docker-compose.infra.yml up -d
./scripts/dahlia-tables.sh
./scripts/dahlia-queues.sh
go test ./... -tags=integration
```

### Code Quality
```bash
# Format code
goimports -w .

# Lint code
golangci-lint run

# Vet code
go vet ./...
```

### Environment Variables
```bash
# AWS LocalStack
AWS_ENDPOINT=http://localhost:4566
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test

# Redis
REDIS_ADDR=localhost:6379

# ZooKeeper
ZK_SERVERS=localhost:2181
```

## 📊 Infrastructure Services

### LocalStack (Port 4566)
- DynamoDB tables: signals, workflows, workflow_runs, action_logs, scheduled_jobs
- SQS queues: executor-queue, executor-dlq

### DynamoDB Local (Port 9000)
- Local DynamoDB instance
- Tables initialized via scripts/dahlia-tables.sh

### Redis (Port 6379)
- Workflow caching
- Absence detection (TTL-based)
- Minute buckets for scheduling
- Keyspace notifications enabled

### ZooKeeper (Port 2181)
- Workflow version coordination
- Cache refresh triggers
- Future: Leader election

## 🚧 Implementation Status

### ✅ Completed
- [x] Health handlers with standardized responses
- [x] Generic HTTP server with FX lifecycle
- [x] Modular provider pattern configuration
- [x] Ultra-minimal main.go files (26 lines each)
- [x] Structured logging with Zap
- [x] VS Code debug configurations
- [x] Docker infrastructure setup

### 🔄 In Progress
- [ ] Domain models & repositories
- [ ] Workflow execution engine
- [ ] Condition evaluators
- [ ] Action executors (Slack/webhook mocks)
- [ ] Scheduler with Redis buckets
- [ ] ZooKeeper coordination
- [ ] API endpoints implementation
