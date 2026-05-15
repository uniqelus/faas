package app

import (
	pkggrpc "github.com/uniqelus/faas/pkg/grpc"
	pkghttp "github.com/uniqelus/faas/pkg/http"
	pkglog "github.com/uniqelus/faas/pkg/log"
	pkgobs "github.com/uniqelus/faas/pkg/observability"
)

type Config struct {
	GrpcServer    pkggrpc.ServerComponentConfig `yaml:"grpc_server"`
	HTTPServer    pkghttp.ServerComponentConfig `yaml:"http_server"`
	Observability pkgobs.Config                 `yaml:"observability"`
	Log           pkglog.Config                 `yaml:"log"`
}
