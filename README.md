<div align="center">

  <img alt="gopher" title="gopher" src="./docs/images/gopher.png" width="325" />

# Tunnel Please

A lightweight SSH-based tunnel server 

<br/><br/>

[![Coverage](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=coverage&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Lines of Code](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=ncloc&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Quality Gate Status](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=alert_status&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Security Issues](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=software_quality_security_issues&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Maintainability Rating](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=software_quality_maintainability_rating&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Reliability Rating](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=software_quality_reliability_rating&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)
[![Security Rating](https://sonar.fossy.my.id/api/project_badges/measure?project=tunnel-please&metric=software_quality_security_rating&token=sqb_0feaed756b943aa75a79499d45d6610c1620830d)](https://sonar.fossy.my.id/dashboard?id=tunnel-please)

</div>

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

| Variable            | Description                                                                 | Default                 | Required            |
|---------------------|-----------------------------------------------------------------------------|-------------------------|---------------------|
| `DOMAIN`            | Domain name for subdomain routing                                           | `localhost`             | No                  |
| `PORT`              | SSH server port                                                             | `2200`                  | No                  |
| `HTTP_PORT`         | HTTP server port                                                            | `8080`                  | No                  |
| `HTTPS_PORT`        | HTTPS server port                                                           | `8443`                  | No                  |
| `KEY_LOC`           | Path to the private key file                                                | `certs/privkey.pem`     | No                  |
| `TLS_ENABLED`       | Enable TLS/HTTPS                                                            | `false`                 | No                  |
| `TLS_REDIRECT`      | Redirect HTTP to HTTPS                                                      | `false`                 | No                  |
| `TLS_STORAGE_PATH`  | Path to store TLS certificates                                             | `certs/tls/`            | No                  |
| `ACME_EMAIL`        | Email for Let's Encrypt registration                                        | `admin@<DOMAIN>`        | No                  |
| `CF_API_TOKEN`      | Cloudflare API token for DNS-01 challenge                                   | `-`                     | Yes (if auto-cert)  |
| `ACME_STAGING`      | Use Let's Encrypt staging server                                            | `false`                 | No                  |
| `CORS_LIST`         | Comma-separated list of allowed CORS origins                                | `-`                     | No                  |
| `ALLOWED_PORTS`     | Port range for TCP tunnels (e.g., 40000-41000)                              | `40000-41000`           | No                  |
| `BUFFER_SIZE`       | Buffer size for io.Copy operations in bytes (4096-1048576)                  | `32768`                 | No                  |
| `MAX_HEADER_SIZE`   | Maximum size of HTTP headers in bytes (4096-131072)                         | `4096`                  | No                  |
| `PPROF_ENABLED`     | Enable pprof profiling server                                               | `false`                 | No                  |
| `PPROF_PORT`        | Port for pprof server                                                       | `6060`                  | No                  |
| `MODE`              | Runtime mode: `standalone` or `node`                                        | `standalone`            | No                  |
| `GRPC_ADDRESS`      | gRPC server address/host used in `node` mode                                | `localhost`             | No                  |
| `GRPC_PORT`         | gRPC server port used in `node` mode                                        | `8080`                  | No                  |
| `NODE_TOKEN`        | Authentication token sent to controller in `node` mode                      | `-`                     | Yes (node mode)     |

**Note:** All environment variables now use UPPERCASE naming. The application includes sensible defaults for all variables, so you can run it without a `.env` file for basic functionality.

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

