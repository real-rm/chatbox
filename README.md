# Chat Application WebSocket

A real-time chat application with WebSocket backend in Go, featuring AI-powered conversations, file uploads, voice messages, and administrative monitoring.

## Project Structure

```
.
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   └── config/          # Configuration management
├── pkg/                 # Public packages (to be added)
├── deployments/
│   └── kubernetes/      # Kubernetes manifests
│       ├── configmap.yaml
│       └── secret.yaml
├── go.mod
└── README.md
```

## Configuration

The application is configured via environment variables and Kubernetes ConfigMaps/Secrets.

### Required Environment Variables

#### Server Configuration
- `SERVER_PORT` - Server port (default: 8080)
- `RECONNECT_TIMEOUT` - Session reconnection timeout (default: 15m)
- `MAX_CONNECTIONS` - Maximum concurrent connections (default: 10000)
- `RATE_LIMIT` - Rate limit per user (default: 100)
- `JWT_SECRET` - JWT signing secret (required)

#### Database Configuration
- `MONGO_URI` - MongoDB connection URI (default: mongodb://localhost:27017)
- `MONGO_DATABASE` - Database name (default: chat)
- `MONGO_COLLECTION` - Collection name (default: sessions)
- `MONGO_CONNECT_TIMEOUT` - Connection timeout (default: 10s)

#### Storage Configuration
- `S3_REGION` - AWS S3 region (required)
- `S3_BUCKET` - S3 bucket name (required)
- `S3_ACCESS_KEY_ID` - AWS access key (required)
- `S3_SECRET_ACCESS_KEY` - AWS secret key (required)
- `S3_ENDPOINT` - Custom S3 endpoint (optional)

#### LLM Provider Configuration
Multiple LLM providers can be configured using numbered environment variables:

```
LLM_PROVIDER_1_ID=openai-gpt4
LLM_PROVIDER_1_NAME=GPT-4
LLM_PROVIDER_1_TYPE=openai
LLM_PROVIDER_1_ENDPOINT=https://api.openai.com/v1
LLM_PROVIDER_1_API_KEY=your-api-key
LLM_PROVIDER_1_MODEL=gpt-4

LLM_PROVIDER_2_ID=anthropic-claude
LLM_PROVIDER_2_NAME=Claude 3
LLM_PROVIDER_2_TYPE=anthropic
LLM_PROVIDER_2_ENDPOINT=https://api.anthropic.com/v1
LLM_PROVIDER_2_API_KEY=your-api-key
LLM_PROVIDER_2_MODEL=claude-3-opus-20240229
```

Supported provider types: `openai`, `anthropic`, `dify`

#### Notification Configuration
- `ADMIN_EMAILS` - Comma-separated admin emails
- `ADMIN_PHONES` - Comma-separated admin phone numbers
- `EMAIL_FROM` - Sender email address
- `SMTP_HOST` - SMTP server host
- `SMTP_PORT` - SMTP server port (default: 587)
- `SMTP_USER` - SMTP username
- `SMTP_PASS` - SMTP password
- `SMS_PROVIDER` - SMS provider name
- `SMS_API_KEY` - SMS API key

## Development

### Prerequisites
- Go 1.21 or higher
- MongoDB
- AWS S3 or compatible storage
- LLM API access (OpenAI, Anthropic, or Dify)

### Running Tests

```bash
go test ./...
```

### Running the Server

```bash
# Set required environment variables
export JWT_SECRET="your-secret"
export S3_ACCESS_KEY_ID="your-key"
export S3_SECRET_ACCESS_KEY="your-secret"
export LLM_PROVIDER_1_ID="openai-gpt4"
export LLM_PROVIDER_1_NAME="GPT-4"
export LLM_PROVIDER_1_TYPE="openai"
export LLM_PROVIDER_1_ENDPOINT="https://api.openai.com/v1"
export LLM_PROVIDER_1_API_KEY="your-api-key"

# Run the server
go run cmd/server/main.go
```

## Kubernetes Deployment

### Apply ConfigMap and Secret

```bash
# Edit the ConfigMap and Secret with your values
kubectl apply -f deployments/kubernetes/configmap.yaml
kubectl apply -f deployments/kubernetes/secret.yaml
```

### Deploy the Application

```bash
# Deployment manifest to be added in later tasks
kubectl apply -f deployments/kubernetes/deployment.yaml
```

## Testing

The project follows TDD principles with comprehensive test coverage:

- **Unit Tests**: Test individual functions and methods
- **Property-Based Tests**: Validate universal correctness properties
- **Integration Tests**: Test end-to-end flows

Run tests with:
```bash
go test -v ./...
```

## License

[Your License Here]
