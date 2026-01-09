# Tunnel Please

A lightweight SSH-based tunnel server written in Go that enables secure TCP and HTTP forwarding with an interactive terminal interface for managing connections and custom subdomains.

## Features

- SSH interactive session with real-time command handling
- Custom subdomain management for HTTP tunnels
- Dual protocol support: HTTP and TCP tunnels
- Real-time connection monitoring
## Requirements

- Go 1.18 or higher
- Valid domain name for subdomain routing

## Environment Variables

The following environment variables can be configured in the `.env` file:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `DOMAIN` | Domain name for subdomain routing | `localhost` | No |
| `PORT` | SSH server port | `2200` | No |
| `HTTP_PORT` | HTTP server port | `8080` | No |
| `HTTPS_PORT` | HTTPS server port | `8443` | No |
| `TLS_ENABLED` | Enable TLS/HTTPS | `false` | No |
| `TLS_REDIRECT` | Redirect HTTP to HTTPS | `false` | No |
| `ACME_EMAIL` | Email for Let's Encrypt registration | `admin@<DOMAIN>` | No |
| `CF_API_TOKEN` | Cloudflare API token for DNS-01 challenge | - | Yes (if auto-cert) |
| `ACME_STAGING` | Use Let's Encrypt staging server | `false` | No |
| `CORS_LIST` | Comma-separated list of allowed CORS origins | - | No |
| `ALLOWED_PORTS` | Port range for TCP tunnels (e.g., 40000-41000) | `40000-41000` | No |
| `BUFFER_SIZE` | Buffer size for io.Copy operations in bytes (4096-1048576) | `32768` | No |
| `PPROF_ENABLED` | Enable pprof profiling server | `false` | No |
| `PPROF_PORT` | Port for pprof server | `6060` | No |
| `MODE` | Runtime mode: `standalone` (default, no gRPC/auth) or `node` (enable gRPC + auth) | `standalone` | No |
| `GRPC_ADDRESS` | gRPC server address/host used in `node` mode | `localhost` | No |
| `GRPC_PORT` | gRPC server port used in `node` mode | `8080` | No |
| `NODE_TOKEN` | Authentication token sent to controller in `node` mode | - (required in `node`) | Yes (node mode) |

**Note:** All environment variables now use UPPERCASE naming. The application includes sensible defaults for all variables, so you can run it without a `.env` file for basic functionality.

### Automatic TLS Certificate Management

The server supports automatic TLS certificate generation and renewal using [CertMagic](https://github.com/caddyserver/certmagic) with Cloudflare DNS-01 challenge. This is required for wildcard certificate support (`*.yourdomain.com`).

**Certificate Storage:**
- TLS certificates are stored in `certs/tls/` (relative to application directory)
- User-provided certificates: `certs/tls/cert.pem` and `certs/tls/privkey.pem`
- CertMagic automatic certificates: `certs/tls/certmagic/`
- SSH keys are stored separately in `certs/ssh/`

**How it works:**
1. If user-provided certificates exist at `certs/tls/cert.pem` and `certs/tls/privkey.pem` and cover both `DOMAIN` and `*.DOMAIN`, they will be used
2. If certificates are missing, expired, expiring within 30 days, or don't cover the required domains, CertMagic will automatically obtain new certificates from Let's Encrypt
3. Certificates are automatically renewed before expiration
4. User-provided certificates support hot-reload (changes detected every 30 seconds)

**Cloudflare API Token Setup:**

To use automatic certificate generation, you need a Cloudflare API token with the following permissions:

1. Go to [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)
2. Click "Create Token"
3. Use "Create Custom Token" with these permissions:
   - **Zone → Zone → Read** (for all zones or specific zone)
   - **Zone → DNS → Edit** (for all zones or specific zone)
4. Copy the token and set it as `CF_API_TOKEN` environment variable

**Example configuration for automatic certificates:**
```env
DOMAIN=example.com
TLS_ENABLED=true
CF_API_TOKEN=your_cloudflare_api_token_here
ACME_EMAIL=admin@example.com
# ACME_STAGING=true  # Uncomment for testing to avoid rate limits
```

### SSH Key Auto-Generation

The application will automatically generate a new 4096-bit RSA key pair at `certs/ssh/id_rsa` if it doesn't exist. This makes it easier to get started without manually creating SSH keys. SSH keys are stored separately from TLS certificates.

### Memory Optimization

The application uses a buffer pool with controlled buffer sizes to prevent excessive memory usage under high concurrent loads. The `BUFFER_SIZE` environment variable controls the size of buffers used for io.Copy operations:

- **Default:** 32768 bytes (32 KB) - Good balance for most scenarios
- **Minimum:** 4096 bytes (4 KB) - Lower memory usage, more CPU overhead
- **Maximum:** 1048576 bytes (1 MB) - Higher throughput, more memory usage

**Recommended settings based on load:**
- **Low traffic (<100 concurrent):** `BUFFER_SIZE=32768` (default)
- **High traffic (>100 concurrent):** `BUFFER_SIZE=16384` or `BUFFER_SIZE=8192`
- **Very high traffic (>1000 concurrent):** `BUFFER_SIZE=8192` or `BUFFER_SIZE=4096`

The buffer pool reuses buffers across connections, preventing memory fragmentation and reducing garbage collection pressure.

### Profiling with pprof

To enable profiling for performance analysis:

1. Set `PPROF_ENABLED=true` in your `.env` file
2. Optionally set `PPROF_PORT` to your desired port (default: 6060)
3. Access profiling data at `http://localhost:6060/debug/pprof/`

Common pprof endpoints:
- `/debug/pprof/` - Index page with available profiles
- `/debug/pprof/heap` - Memory allocation profile
- `/debug/pprof/goroutine` - Stack traces of all current goroutines
- `/debug/pprof/profile` - CPU profile (30-second sample by default)
- `/debug/pprof/trace` - Execution trace

Example usage with `go tool pprof`:
```bash
# Analyze CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Analyze memory heap
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Docker Deployment

Three Docker Compose configurations are available for different deployment scenarios. Each configuration uses the image `git.fossy.my.id/bagas/tunnel-please:latest`.

### Configuration Options

#### 1. Root with Host Networking (RECOMMENDED)

**File:** `docker-compose.root.yml`

**Advantages:**
- Full TCP port forwarding support (ports 40000-41000)
- Direct binding to privileged ports (80, 443, 2200)
- Best performance with no NAT overhead
- Maximum flexibility for all tunnel types
- No port mapping limitations

**Use Case:** Production deployments where you need unrestricted TCP forwarding and maximum performance.

**Deploy:**
```bash
docker-compose -f docker-compose.root.yml up -d
```

#### 2. Standard (HTTP/HTTPS Only)

**File:** `docker-compose.standard.yml`

**Advantages:**
- Runs with unprivileged user (more secure)
- Standard port mappings (2200, 80, 443)
- Simple and predictable networking
- TCP port forwarding disabled (`ALLOWED_PORTS=none`)

**Use Case:** Deployments where you only need HTTP/HTTPS tunneling without custom TCP port forwarding.

**Deploy:**
```bash
docker-compose -f docker-compose.standard.yml up -d
```

#### 3. Limited TCP Forwarding

**File:** `docker-compose.tcp.yml`

**Advantages:**
- Runs with unprivileged user (more secure)
- Standard port mappings (2200, 80, 443)
- Limited TCP forwarding (ports 30000-31000)
- Controlled port range exposure

**Use Case:** Deployments where you need both HTTP/HTTPS tunneling and limited TCP forwarding within a specific port range.

**Deploy:**
```bash
docker-compose -f docker-compose.tcp.yml up -d
```

### Quick Start

1. **Choose your configuration** based on your requirements
2. **Edit the environment variables** in the chosen compose file:
   - `DOMAIN`: Your domain name (e.g., `example.com`)
   - `ACME_EMAIL`: Your email for Let's Encrypt
   - `CF_API_TOKEN`: Your Cloudflare API token (if using automatic TLS)
3. **Deploy:**
   ```bash
   docker-compose -f docker-compose.root.yml up -d
   ```
4. **Check logs:**
   ```bash
   docker-compose -f docker-compose.root.yml logs -f
   ```
5. **Stop the service:**
   ```bash
   docker-compose -f docker-compose.root.yml down
   ```

### Volume Management

All configurations use a named volume `certs` for persistent storage:
- SSH keys: `/app/certs/ssh/`
- TLS certificates: `/app/certs/tls/`

To backup certificates:
```bash
docker run --rm -v tunnel_pls_certs:/data -v $(pwd):/backup alpine tar czf /backup/certs-backup.tar.gz -C /data .
```

To restore certificates:
```bash
docker run --rm -v tunnel_pls_certs:/data -v $(pwd):/backup alpine tar xzf /backup/certs-backup.tar.gz -C /data
```

### Recommendation

**Use `docker-compose.root.yml`** for production deployments if you need:
- Full TCP port forwarding capabilities
- Any port range configuration
- Direct port binding without mapping overhead
- Maximum performance and flexibility

This is the recommended configuration for most use cases as it provides the complete feature set without limitations.

## Contributing
Contributions are welcome!

If you'd like to contribute to this project, please follow the workflow below:

1. **Fork** the repository
2. Create a new branch for your changes
3. Commit and push your updates
4. Open a **Pull Request** targeting the **`staging`** branch
5. Clearly describe your changes and the reasoning behind them

## License
This project is licensed under the [Attribution-NonCommercial-NoDerivatives 4.0 International (CC BY-NC-ND 4.0)](https://creativecommons.org/licenses/by-nc-nd/4.0/) license.
## Author
**Bagas (fossyy)**

- Website: [fossy.my.id](https://fossy.my.id)
- GitHub: [@fossyy](https://github.com/fossyy)

