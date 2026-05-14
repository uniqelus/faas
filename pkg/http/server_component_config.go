package pkghttp

import "time"

type ServerComponentConfig struct {
	Host              string        `yaml:"host" env:"HTTP_HOST" env-default:"0.0.0.0"`
	Port              string        `yaml:"port" env:"HTTP_PORT" env-default:"8080"`
	AdminHost         string        `yaml:"admin_host" env:"HTTP_ADMIN_HOST" env-default:"0.0.0.0"`
	AdminPort         string        `yaml:"admin_port" env:"HTTP_ADMIN_PORT" env-default:"8081"`
	AdminDisabled     bool          `yaml:"admin_disabled" env:"HTTP_ADMIN_DISABLED" env-default:"false"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout" env:"HTTP_READ_HEADER_TIMEOUT" env-default:"5s"`
	ReadTimeout       time.Duration `yaml:"read_timeout" env:"HTTP_READ_TIMEOUT" env-default:"30s"`
	WriteTimeout      time.Duration `yaml:"write_timeout" env:"HTTP_WRITE_TIMEOUT" env-default:"30s"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" env:"HTTP_IDLE_TIMEOUT" env-default:"120s"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout" env:"HTTP_SHUTDOWN_TIMEOUT" env-default:"30s"`
}

func (c ServerComponentConfig) Options() []ServerComponentOption {
	return []ServerComponentOption{
		WithHost(c.Host),
		WithPort(c.Port),
		WithAdminHost(c.AdminHost),
		WithAdminPort(c.AdminPort),
		WithAdminDisabled(c.AdminDisabled),
		WithReadHeaderTimeout(c.ReadHeaderTimeout),
		WithReadTimeout(c.ReadTimeout),
		WithWriteTimeout(c.WriteTimeout),
		WithIdleTimeout(c.IdleTimeout),
		WithShutdownTimeout(c.ShutdownTimeout),
	}
}
