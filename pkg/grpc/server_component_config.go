package pkggrpc

type ServerComponentConfig struct {
	Host string `yaml:"host" env:"HOST" env-default:"0.0.0.0"`
	Port string `yaml:"port" env:"PORT" env-default:"20000"`
}

func (c ServerComponentConfig) Options() []ServerComponentOption {
	return []ServerComponentOption{
		WithHost(c.Host),
		WithPort(c.Port),
	}
}
