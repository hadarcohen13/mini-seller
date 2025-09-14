# Mini Seller - Bid Request Server

A Go-based server for handling bid requests following clean architecture principles.

## Project Structure

```
mini-seller/
├── cmd/
│   └── server/          # Application entrypoints
├── internal/            # Private application code
│   ├── config/         # Configuration management
│   ├── handlers/       # HTTP handlers
│   ├── middleware/     # HTTP middleware
│   ├── models/         # Data models
│   └── services/       # Business logic
├── pkg/                # Public library code
│   └── utils/          # Utility functions
├── api/                # API definitions (OpenAPI/Swagger)
├── deployments/        # Docker, k8s configs
├── scripts/           # Build and deployment scripts
├── go.mod             # Go module definition
└── main.go            # Temporary main (will be moved to cmd/server)
```

## Architecture Principles

- **cmd/**: Contains the main applications for this project
- **internal/**: Private application and library code
- **pkg/**: Library code that can be used by external applications
- **api/**: API contract definitions
- **deployments/**: System and container orchestration deployments
- **scripts/**: Scripts for various build, install, analysis operations

## Getting Started

1. Initialize dependencies: `go mod tidy`
2. Run the server: `go run cmd/server/main.go`
3. Server will start on port 8080 by default