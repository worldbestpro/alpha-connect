package coinbasepro

import (
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
)

// Execute api calls
// Contains rate limit
// Spawn a query actor for each request
// and pipe its result back

// 429 rate limit
// 418 IP ban

// The role of a CoinbasePro Executor is to
// process api request
type Executor struct {
	privateExecutor *actor.PID
	publicExecutor  *actor.PID
	fixExecutor     *actor.PID
	logger          *log.Logger
}

func NewExecutor() actor.Actor {
	return &Executor{
		privateExecutor: nil,
		publicExecutor:  nil,
		fixExecutor:     nil,
		logger:          nil,
	}
}

func (state *Executor) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		state.publicExecutor = context.Spawn(actor.PropsFromProducer(NewPublicExecutor))
		state.privateExecutor = context.Spawn(actor.PropsFromProducer(NewPrivateExecutor))
		state.fixExecutor = context.Spawn(actor.PropsFromProducer(NewFixExecutor))

	case *messages.SecurityList:
		context.Forward(context.Parent())

	case *messages.SecurityListRequest,
		*messages.SecurityDefinitionRequest,
		*messages.MarketableProtocolAssetListRequest,
		*messages.HistoricalLiquidationsRequest,
		*messages.MarketDataRequest:
		context.Forward(state.publicExecutor)
	}
}
