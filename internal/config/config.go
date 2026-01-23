package config

import "tunnel_pls/types"

type Config interface {
	Domain() string
	SSHPort() string

	HTTPPort() string
	HTTPSPort() string

	KeyLoc() string

	TLSEnabled() bool
	TLSRedirect() bool

	ACMEEmail() string
	CFAPIToken() string
	ACMEStaging() bool

	AllowedPortsStart() uint16
	AllowedPortsEnd() uint16

	BufferSize() int

	PprofEnabled() bool
	PprofPort() string

	Mode() types.ServerMode
	GRPCAddress() string
	GRPCPort() string
	NodeToken() string
}

func MustLoad() (Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	cfg, err := parse()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *config) Domain() string            { return c.domain }
func (c *config) SSHPort() string           { return c.sshPort }
func (c *config) HTTPPort() string          { return c.httpPort }
func (c *config) HTTPSPort() string         { return c.httpsPort }
func (c *config) KeyLoc() string            { return c.keyLoc }
func (c *config) TLSEnabled() bool          { return c.tlsEnabled }
func (c *config) TLSRedirect() bool         { return c.tlsRedirect }
func (c *config) ACMEEmail() string         { return c.acmeEmail }
func (c *config) CFAPIToken() string        { return c.cfAPIToken }
func (c *config) ACMEStaging() bool         { return c.acmeStaging }
func (c *config) AllowedPortsStart() uint16 { return c.allowedPortsStart }
func (c *config) AllowedPortsEnd() uint16   { return c.allowedPortsEnd }
func (c *config) BufferSize() int           { return c.bufferSize }
func (c *config) PprofEnabled() bool        { return c.pprofEnabled }
func (c *config) PprofPort() string         { return c.pprofPort }
func (c *config) Mode() types.ServerMode    { return c.mode }
func (c *config) GRPCAddress() string       { return c.grpcAddress }
func (c *config) GRPCPort() string          { return c.grpcPort }
func (c *config) NodeToken() string         { return c.nodeToken }
