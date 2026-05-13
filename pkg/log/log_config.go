package pkglog

type Config struct {
	Level    string `yaml:"level" env:"LOG_LEVEL" env-default:"info"`
	Encoding string `yaml:"encoding" env:"LOG_ENCODING" env-default:"json"`
}

func (c Config) Options() []Option {
	return []Option{
		WithLevel(c.Level),
		WithEncoding(c.Encoding),
	}
}
