package app

import (
	pkggrpc "github.com/uniqelus/faas/pkg/grpc"
	pkglog "github.com/uniqelus/faas/pkg/log"
)

type Config struct {
	GrpcServer pkggrpc.ServerComponentConfig `yaml:"grpc_server"`
	Log        pkglog.Config                 `yaml:"log"`
}
