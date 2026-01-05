# IMT Cloud CI/CD Backend

A lightweight CI/CD engine built in Go that executes pipelines using Docker containers.

## Features

- 🔗 **GitHub Webhook Integration** - Automatically trigger pipelines on push
- 📄 **GitLab CI Config Parser** - Parse `.gitlab-ci.yml` configuration files
- 🐳 **Docker Execution** - Run jobs in isolated Docker containers
- 💾 **PostgreSQL Storage** - Store pipelines, jobs, and logs
- 📊 **Database Viewer** - Adminer for easy data inspection

## Architecture

```
GitHub Push → Webhook → Clone Repo → Parse .gitlab-ci.yml → Run Jobs → Store Logs
```

## Prerequisites

- Docker
- Docker Compose

## Quick Start

### 1. Start the services

```bash
docker-compose up -d
```

This will start:
- **cicd-backend** - The CI/CD engine on port `8080`
- **cicd-db** - PostgreSQL database on port `5432`
- **cicd-adminer** - Database viewer on port `8081`

### 2. Access the services

| Service | URL | Credentials |
|---------|-----|-------------|
| Webhook Endpoint | http://localhost:8080/webhook/github | - |
| Health Check | http://localhost:8080/health | - |
| Adminer (DB Viewer) | http://localhost:8081 | System: PostgreSQL, Server: db, User: cicd, Password: cicd_password, Database: cicd_db |

### 3. View logs

```bash
docker-compose logs -f backend
```

## GitHub Webhook Setup

### 1. Expose your backend (for local development)

Use [ngrok](https://ngrok.com/) or similar to expose your local server:

```bash
ngrok http 8080
```

This gives you a public URL like `https://abc123.ngrok.io`

### 2. Configure GitHub Webhook

1. Go to your GitHub repository → **Settings** → **Webhooks** → **Add webhook**
2. Configure:
   - **Payload URL**: `https://abc123.ngrok.io/webhook/github`
   - **Content type**: `application/json`
   - **Secret**: (optional, not implemented yet)
   - **Events**: Select "Just the push event"
3. Click **Add webhook**

### 3. Add a `.gitlab-ci.yml` to your repo

```yaml
stages:
  - build
  - test

build_job:
  stage: build
  image: alpine:latest
  script:
    - echo "Building the project..."
    - ls -la

test_job:
  stage: test
  image: alpine:latest
  script:
    - echo "Running tests..."
    - echo "All tests passed!"
```

### 4. Push code and watch the magic!

```bash
git add .
git commit -m "Add CI config"
git push
```

The backend will:
1. Receive the webhook from GitHub
2. Clone your repository
3. Parse the `.gitlab-ci.yml`
4. Execute each job in Docker containers
5. Store all logs in the database

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://cicd:cicd_password@db:5432/cicd_db?sslmode=disable` |
| `DOCKER_HOST` | Docker socket path | `unix:///var/run/docker.sock` |
| `API_PORT` | Server port | `8080` |

## API Endpoints

### Webhook

```
POST /webhook/github
```

Receives GitHub push events and triggers pipelines.

**Headers:**
- `X-GitHub-Event: push`

**Response:**
```json
{
  "message": "Pipeline triggered",
  "branch": "main",
  "commit": "abc123..."
}
```

### Health Check

```
GET /health
```

Returns server status.

**Response:**
```json
{
  "status": "ok"
}
```

## Database Schema

| Table | Description |
|-------|-------------|
| `projects` | Repository configurations |
| `pipelines` | Pipeline execution records |
| `jobs` | Individual job records within pipelines |
| `logs` | Line-by-line log storage |

## Supported CI Config

The parser supports a subset of GitLab CI syntax:

```yaml
stages:
  - build
  - test
  - deploy

job_name:
  stage: build
  image: node:18-alpine
  script:
    - npm install
    - npm run build
```

## Development

### Build locally

```bash
go mod tidy
go build -o cicd-backend .
```

### Run locally

```bash
export DATABASE_URL="postgres://cicd:cicd_password@localhost:5432/cicd_db?sslmode=disable"
./cicd-backend
```

### Rebuild Docker image

```bash
docker-compose build --no-cache backend
docker-compose up -d
```

## Stopping the services

```bash
docker-compose down
```

To also remove the database volume:

```bash
docker-compose down -v
```

## Project Structure

```
.
├── main.go                 # Entry point - starts API server
├── internal/
│   ├── api/
│   │   ├── server.go       # HTTP server and handlers
│   │   └── webhook.go      # GitHub webhook payload types
│   ├── database/
│   │   └── database.go     # PostgreSQL operations
│   ├── executor/
│   │   └── executor.go     # Docker job execution
│   ├── git/
│   │   └── git.go          # Git clone operations
│   ├── models/
│   │   └── models.go       # Data models
│   └── parser/
│       └── parser.go       # YAML config parser
├── Dockerfile
├── docker-compose.yml
├── init-db.sql
└── schema.sql
```

## License

MIT