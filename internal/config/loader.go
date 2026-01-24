package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"tunnel_pls/types"

	"github.com/joho/godotenv"
)

type config struct {
	domain  string
	sshPort string

	httpPort  string
	httpsPort string

	keyLoc string

	tlsEnabled     bool
	tlsRedirect    bool
	tlsStoragePath string
	acmeEmail      string
	cfAPIToken     string
	acmeStaging    bool

	allowedPortsStart uint16
	allowedPortsEnd   uint16

	bufferSize int

	pprofEnabled bool
	pprofPort    string

	mode        types.ServerMode
	grpcAddress string
	grpcPort    string
	nodeToken   string
}

func parse() (*config, error) {
	mode, err := parseMode()
	if err != nil {
		return nil, err
	}

	domain := getenv("DOMAIN", "localhost")
	sshPort := getenv("PORT", "2200")

	httpPort := getenv("HTTP_PORT", "8080")
	httpsPort := getenv("HTTPS_PORT", "8443")

	keyLoc := getenv("KEY_LOC", "certs/privkey.pem")

	tlsEnabled := getenvBool("TLS_ENABLED", false)
	tlsRedirect := tlsEnabled && getenvBool("TLS_REDIRECT", false)
	tlsStoragePath := getenv("TLS_STORAGE_PATH", "certs/tls/")

	acmeEmail := getenv("ACME_EMAIL", "admin@"+domain)
	acmeStaging := getenvBool("ACME_STAGING", false)

	cfToken := getenv("CF_API_TOKEN", "")
	if tlsEnabled && cfToken == "" {
		return nil, fmt.Errorf("CF_API_TOKEN is required when TLS is enabled")
	}

	start, end, err := parseAllowedPorts()
	if err != nil {
		return nil, err
	}

	bufferSize := parseBufferSize()

	pprofEnabled := getenvBool("PPROF_ENABLED", false)
	pprofPort := getenv("PPROF_PORT", "6060")

	grpcHost := getenv("GRPC_ADDRESS", "localhost")
	grpcPort := getenv("GRPC_PORT", "8080")

	nodeToken := getenv("NODE_TOKEN", "")
	if mode == types.ServerModeNODE && nodeToken == "" {
		return nil, fmt.Errorf("NODE_TOKEN is required in node mode")
	}

	return &config{
		domain:            domain,
		sshPort:           sshPort,
		httpPort:          httpPort,
		httpsPort:         httpsPort,
		keyLoc:            keyLoc,
		tlsEnabled:        tlsEnabled,
		tlsRedirect:       tlsRedirect,
		tlsStoragePath:    tlsStoragePath,
		acmeEmail:         acmeEmail,
		cfAPIToken:        cfToken,
		acmeStaging:       acmeStaging,
		allowedPortsStart: start,
		allowedPortsEnd:   end,
		bufferSize:        bufferSize,
		pprofEnabled:      pprofEnabled,
		pprofPort:         pprofPort,
		mode:              mode,
		grpcAddress:       grpcHost,
		grpcPort:          grpcPort,
		nodeToken:         nodeToken,
	}, nil
}

func loadEnvFile() error {
	if _, err := os.Stat(".env"); err == nil {
		return godotenv.Load(".env")
	}
	return nil
}

func parseMode() (types.ServerMode, error) {
	switch strings.ToLower(getenv("MODE", "standalone")) {
	case "standalone":
		return types.ServerModeSTANDALONE, nil
	case "node":
		return types.ServerModeNODE, nil
	default:
		return 0, fmt.Errorf("invalid MODE value")
	}
}

func parseAllowedPorts() (uint16, uint16, error) {
	raw := getenv("ALLOWED_PORTS", "")
	if raw == "" {
		return 0, 0, nil
	}

	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid ALLOWED_PORTS format")
	}

	start, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return 0, 0, err
	}

	end, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return 0, 0, err
	}

	return uint16(start), uint16(end), nil
}

func parseBufferSize() int {
	raw := getenv("BUFFER_SIZE", "32768")
	size, err := strconv.Atoi(raw)
	if err != nil || size < 4096 || size > 1048576 {
		log.Println("Invalid BUFFER_SIZE, falling back to 4096")
		return 4096
	}
	return size
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val == "true"
}
