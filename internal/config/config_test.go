package config

import (
	"os"
	"testing"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
)

func TestGetenv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		val      string
		def      string
		expected string
	}{
		{
			name:     "returns existing env",
			key:      "TEST_ENV_EXIST",
			val:      "value",
			def:      "default",
			expected: "value",
		},
		{
			name:     "returns default when env missing",
			key:      "TEST_ENV_MISSING",
			val:      "",
			def:      "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val != "" {
				t.Setenv(tt.key, tt.val)
			} else {
				os.Unsetenv(tt.key)
			}
			assert.Equal(t, tt.expected, getenv(tt.key, tt.def))
		})
	}
}

func TestGetenvBool(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		val      string
		def      bool
		expected bool
	}{
		{
			name:     "returns true when env is true",
			key:      "TEST_BOOL_TRUE",
			val:      "true",
			def:      false,
			expected: true,
		},
		{
			name:     "returns false when env is false",
			key:      "TEST_BOOL_FALSE",
			val:      "false",
			def:      true,
			expected: false,
		},
		{
			name:     "returns default when env missing",
			key:      "TEST_BOOL_MISSING",
			val:      "",
			def:      true,
			expected: true,
		},
		{
			name:     "returns false when env is not true",
			key:      "TEST_BOOL_INVALID",
			val:      "yes",
			def:      true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val != "" {
				t.Setenv(tt.key, tt.val)
			} else {
				os.Unsetenv(tt.key)
			}
			assert.Equal(t, tt.expected, getenvBool(tt.key, tt.def))
		})
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		expect    types.ServerMode
		expectErr bool
	}{
		{"standalone", "standalone", types.ServerModeSTANDALONE, false},
		{"node", "node", types.ServerModeNODE, false},
		{"uppercase", "STANDALONE", types.ServerModeSTANDALONE, false},
		{"invalid", "invalid", 0, true},
		{"empty (default)", "", types.ServerModeSTANDALONE, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mode != "" {
				t.Setenv("MODE", tt.mode)
			} else {
				os.Unsetenv("MODE")
			}
			mode, err := parseMode()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expect, mode)
			}
		})
	}
}

func TestParseAllowedPorts(t *testing.T) {
	tests := []struct {
		name      string
		val       string
		start     uint16
		end       uint16
		expectErr bool
	}{
		{"valid range", "1000-2000", 1000, 2000, false},
		{"empty", "", 0, 0, false},
		{"invalid format - no dash", "1000", 0, 0, true},
		{"invalid format - too many dashes", "1000-2000-3000", 0, 0, true},
		{"invalid start port", "abc-2000", 0, 0, true},
		{"invalid end port", "1000-abc", 0, 0, true},
		{"out of range start", "70000-80000", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val != "" {
				t.Setenv("ALLOWED_PORTS", tt.val)
			} else {
				os.Unsetenv("ALLOWED_PORTS")
			}
			start, end, err := parseAllowedPorts()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.start, start)
				assert.Equal(t, tt.end, end)
			}
		})
	}
}

func TestParseBufferSize(t *testing.T) {
	tests := []struct {
		name   string
		val    string
		expect int
	}{
		{"valid size", "8192", 8192},
		{"default size", "", 32768},
		{"too small", "1024", 4096},
		{"too large", "2000000", 4096},
		{"invalid format", "abc", 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val != "" {
				t.Setenv("BUFFER_SIZE", tt.val)
			} else {
				os.Unsetenv("BUFFER_SIZE")
			}
			size := parseBufferSize()
			assert.Equal(t, tt.expect, size)
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		envs      map[string]string
		expectErr bool
	}{
		{
			name: "minimal valid config",
			envs: map[string]string{
				"DOMAIN": "example.com",
			},
			expectErr: false,
		},
		{
			name: "TLS enabled without token",
			envs: map[string]string{
				"TLS_ENABLED": "true",
			},
			expectErr: true,
		},
		{
			name: "TLS enabled with token",
			envs: map[string]string{
				"TLS_ENABLED":  "true",
				"CF_API_TOKEN": "secret",
			},
			expectErr: false,
		},
		{
			name: "Node mode without token",
			envs: map[string]string{
				"MODE": "node",
			},
			expectErr: true,
		},
		{
			name: "Node mode with token",
			envs: map[string]string{
				"MODE":       "node",
				"NODE_TOKEN": "token",
			},
			expectErr: false,
		},
		{
			name: "invalid mode",
			envs: map[string]string{
				"MODE": "invalid",
			},
			expectErr: true,
		},
		{
			name: "invalid allowed ports",
			envs: map[string]string{
				"ALLOWED_PORTS": "1000",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}
			cfg, err := parse()
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestGetters(t *testing.T) {
	envs := map[string]string{
		"DOMAIN":        "example.com",
		"PORT":          "2222",
		"HTTP_PORT":     "80",
		"HTTPS_PORT":    "443",
		"TLS_ENABLED":   "true",
		"TLS_REDIRECT":  "true",
		"ACME_EMAIL":    "test@example.com",
		"CF_API_TOKEN":  "token",
		"ACME_STAGING":  "true",
		"ALLOWED_PORTS": "1000-2000",
		"BUFFER_SIZE":   "16384",
		"PPROF_ENABLED": "true",
		"PPROF_PORT":    "7070",
		"MODE":          "standalone",
		"GRPC_ADDRESS":  "127.0.0.1",
		"GRPC_PORT":     "9090",
		"NODE_TOKEN":    "ntoken",
	}

	os.Clearenv()
	for k, v := range envs {
		t.Setenv(k, v)
	}

	cfg, err := parse()
	assert.NoError(t, err)

	assert.Equal(t, "example.com", cfg.Domain())
	assert.Equal(t, "2222", cfg.SSHPort())
	assert.Equal(t, "80", cfg.HTTPPort())
	assert.Equal(t, "443", cfg.HTTPSPort())
	assert.Equal(t, true, cfg.TLSEnabled())
	assert.Equal(t, true, cfg.TLSRedirect())
	assert.Equal(t, "test@example.com", cfg.ACMEEmail())
	assert.Equal(t, "token", cfg.CFAPIToken())
	assert.Equal(t, true, cfg.ACMEStaging())
	assert.Equal(t, uint16(1000), cfg.AllowedPortsStart())
	assert.Equal(t, uint16(2000), cfg.AllowedPortsEnd())
	assert.Equal(t, 16384, cfg.BufferSize())
	assert.Equal(t, true, cfg.PprofEnabled())
	assert.Equal(t, "7070", cfg.PprofPort())
	assert.Equal(t, types.ServerMode(types.ServerModeSTANDALONE), cfg.Mode())
	assert.Equal(t, "127.0.0.1", cfg.GRPCAddress())
	assert.Equal(t, "9090", cfg.GRPCPort())
	assert.Equal(t, "ntoken", cfg.NodeToken())
}

func TestMustLoad(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("DOMAIN", "example.com")
		cfg, err := MustLoad()
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
	})

	t.Run("loadEnvFile error", func(t *testing.T) {
		err := os.Mkdir(".env", 0755)
		assert.NoError(t, err)
		defer os.Remove(".env")

		cfg, err := MustLoad()
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("parse error", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("MODE", "invalid")
		cfg, err := MustLoad()
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})
}

func TestLoadEnvFile(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		err := os.WriteFile(".env", []byte("TEST_ENV_FILE=true"), 0644)
		assert.NoError(t, err)
		defer os.Remove(".env")

		err = loadEnvFile()
		assert.NoError(t, err)
		assert.Equal(t, "true", os.Getenv("TEST_ENV_FILE"))
	})

	t.Run("file missing", func(t *testing.T) {
		_ = os.Remove(".env")
		err := loadEnvFile()
		assert.NoError(t, err)
	})
}
