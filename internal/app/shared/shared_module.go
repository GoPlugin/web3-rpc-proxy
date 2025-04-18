package shared

import (
	"github.com/GoPlugin/web3rpcproxy/internal/app/database"
	"go.uber.org/fx"
)

var NewSharedModule = fx.Options(
	fx.Provide(NewEtcdClient),
	fx.Provide(NewConfInstance),
	fx.Provide(NewLogger),

	fx.Provide(NewTransport),
	fx.Provide(NewWatcherClientInstance),

	fx.Provide(database.NewDatabase),
	fx.Provide(NewRedisClient),
	fx.Provide(NewRedisScripts),
	fx.Provide(NewRabbitMQ),
)
