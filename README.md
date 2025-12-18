# Tunnel Please

A lightweight SSH-based tunnel server written in Go that enables secure TCP and HTTP forwarding with an interactive terminal interface for managing connections and custom subdomains.

## Features

- SSH interactive session with real-time command handling
- Custom subdomain management for HTTP tunnels
- Active connection control with drop functionality
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
| `TLS_ENABLED` | Enable TLS/HTTPS | `false` | No |
| `TLS_REDIRECT` | Redirect HTTP to HTTPS | `false` | No |
| `CERT_LOC` | Path to TLS certificate | `certs/cert.pem` | No |
| `KEY_LOC` | Path to TLS private key | `certs/privkey.pem` | No |
| `SSH_PRIVATE_KEY` | Path to SSH private key (auto-generated if missing) | `certs/id_rsa` | No |
| `CORS_LIST` | Comma-separated list of allowed CORS origins | - | No |
| `ALLOWED_PORTS` | Port range for TCP tunnels (e.g., 40000-41000) | `40000-41000` | No |
| `PPROF_ENABLED` | Enable pprof profiling server | `false` | No |
| `PPROF_PORT` | Port for pprof server | `6060` | No |

**Note:** All environment variables now use UPPERCASE naming. The application includes sensible defaults for all variables, so you can run it without a `.env` file for basic functionality.

### SSH Key Auto-Generation

If the SSH private key specified in `SSH_PRIVATE_KEY` doesn't exist, the application will automatically generate a new 4096-bit RSA key pair at the specified location. This makes it easier to get started without manually creating SSH keys.

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

