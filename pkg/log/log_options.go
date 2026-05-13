package pkglog

const (
	DefaultLogLevel = "info"
	DefaultEncoding = "json"
)

type options struct {
	level    string
	encoding string
}

func defaultOptions() []Option {
	return []Option{
		WithLevel(DefaultLogLevel),
		WithEncoding(DefaultEncoding),
	}
}

func newOptions(opts ...Option) *options {
	options := &options{}

	toApply := append(defaultOptions(), opts...)
	for _, opt := range toApply {
		opt(options)
	}

	return options
}

type Option func(*options)

func WithLevel(level string) Option {
	return func(opts *options) {
		opts.level = level
	}
}

func WithEncoding(encoding string) Option {
	return func(opts *options) {
		opts.encoding = encoding
	}
}
