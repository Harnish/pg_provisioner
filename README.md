# PostgreSQL Database Provisioner

A Go-based application that automates PostgreSQL database and user provisioning. It runs in a Docker container and creates databases with dedicated users based on a configuration file.

## Features

- Creates PostgreSQL users with specified passwords
- Creates databases with specified owners
- Sets proper ownership and privileges
- Handles existing databases and users (updates passwords and ownership)
- Connection retry logic for reliability
- Configurable via JSON
- **Watch mode**: Continuously monitors config file for changes
- **Kubernetes native**: Works seamlessly with ConfigMaps
- **Idempotent**: Safe to run multiple times

## Project Structure

```
.
├── main.go              # Go application
├── go.mod               # Go module definition
├── go.sum               # Go dependencies checksum
├── Dockerfile           # Container definition
├── docker-compose.yml   # Example deployment
├── config.json          # Configuration file
└── README.md           # This file
```

## Configuration

Create a `config.json` file with your PostgreSQL connection details and database configurations:

```json
{
  "root_connection_string": "postgres://postgres:rootpassword@postgres:5432/postgres?sslmode=disable",
  "databases": [
    {
      "database": "app_db",
      "user": "app_user",
      "password": "securepassword123"
    }
  ]
}
```

### Configuration Fields

- `root_connection_string`: PostgreSQL connection string with superuser credentials
- `databases`: Array of database configurations
  - `database`: Name of the database to create
  - `user`: Username to create/manage
  - `password`: Password for the user

## Usage

### Using Docker Compose (Recommended)

1. Create your `config.json` file
2. Run the stack:

```bash
docker-compose up --build
```

This will:
- Start a PostgreSQL container
- Build and run the provisioner
- Create all configured databases and users

### Using Docker Directly

1. Build the image:

```bash
docker build -t pg-provisioner .
```

2. Run the container (assuming PostgreSQL is accessible):

```bash
docker run -v $(pwd)/config.json:/config/config.json:ro pg-provisioner
```

### Using Kubernetes with ConfigMaps

The application supports Kubernetes ConfigMaps and can run in two modes:

#### 1. Deployment Mode (Continuous Watch)

Deploy as a long-running pod that watches for ConfigMap changes:

```bash
kubectl apply -f kubernetes-deployment.yaml
```

When you update the ConfigMap:
```bash
kubectl edit configmap pg-provisioner-config
```

The pod will automatically detect changes within ~10 seconds and reprocess the configuration.

**Note**: Kubernetes ConfigMap updates can take 60+ seconds to propagate to mounted volumes. For faster updates, consider using a Job or CronJob approach.

#### 2. Job Mode (Run Once)

Run as a one-time Job:

```bash
kubectl apply -f kubernetes-job.yaml
```

This is useful for:
- Initial database setup
- Scheduled provisioning with CronJobs
- CI/CD pipelines

#### 3. Using Secrets (Recommended for Production)

For sensitive credentials, use Kubernetes Secrets:

```bash
kubectl apply -f kubernetes-with-secrets.yaml
```

This approach:
- Stores connection strings and passwords in Secrets
- Uses an init container to interpolate values
- Keeps the ConfigMap for non-sensitive configuration

### Editing Config on the Fly

#### Docker Compose
Edit `config.json` and set `WATCH_MODE=true`:
```yaml
environment:
  WATCH_MODE: "true"
```
The container will detect changes within 10 seconds.

#### Kubernetes
Update the ConfigMap:
```bash
kubectl edit configmap pg-provisioner-config
# or
kubectl apply -f updated-config.yaml
```

The pod will automatically detect and apply changes.

### Running Locally

1. Install dependencies:

```bash
go mod download
```

2. Run the application:

```bash
# Run once
CONFIG_PATH=./config.json go run main.go

# Run in watch mode
WATCH_MODE=true CONFIG_PATH=./config.json go run main.go
```

## Connection String Format

The root connection string should follow this format:

```
postgres://username:password@host:port/database?sslmode=disable
```

Example:
```
postgres://postgres:mypassword@localhost:5432/postgres?sslmode=disable
```

## Behavior

- **Idempotent**: Safe to run multiple times
- If a user exists: Updates the password
- If a database exists: Updates the owner
- Grants all privileges on the database to the user
- Includes connection retry logic (5 attempts with 5-second delays)

## Environment Variables

- `CONFIG_PATH`: Path to configuration file (default: `/config/config.json`)
- `WATCH_MODE`: Enable continuous monitoring of config file changes (default: `false`)
  - `true`: Monitor config file for changes and reprocess automatically
  - `false`: Run once and exit

## Security Considerations

1. **Protect config.json**: Contains sensitive credentials
2. **Use strong passwords**: Especially for the root connection
3. **Enable SSL**: In production, use `sslmode=require` in connection strings
4. **Limit network access**: Don't expose PostgreSQL directly to the internet
5. **Use secrets management**: Consider using Docker secrets or environment variables for sensitive data

## Error Handling

The application includes comprehensive error handling:
- Connection retry logic
- Validation of configuration
- Detailed logging of operations
- Continues processing remaining databases if one fails

## Logs

The application provides detailed logging:
- Connection status
- User creation/update
- Database creation/update
- Ownership and privilege changes
- Error messages with context

## License

MIT License