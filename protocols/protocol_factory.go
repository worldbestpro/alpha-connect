package protocols

import (
	"github.com/asynkron/protoactor-go/actor"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/protocols/erc20"
	"gitlab.com/alphaticks/alpha-connect/protocols/erc721"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	"gitlab.com/alphaticks/xchanger/constants"
	models2 "gitlab.com/alphaticks/xchanger/models"
)

func NewProtocolExecutorProducer(protocol *models2.Protocol, registry registry.StaticClient) actor.Producer {
	switch protocol.ID {
	case constants.ERC20.ID:
		return func() actor.Actor { return erc20.NewExecutor(registry, protocol) }
	case constants.ERC721.ID:
		return func() actor.Actor { return erc721.NewExecutor(registry, protocol) }
	default:
		return nil
	}
}

func NewProtocolAssetListenerProducer(protocolAsset *models.ProtocolAsset) actor.Producer {
	switch protocolAsset.Protocol.ID {
	case constants.ERC20.ID:
		return func() actor.Actor { return erc20.NewListener(protocolAsset) }
	case constants.ERC721.ID:
		return func() actor.Actor { return erc721.NewListener(protocolAsset) }
	}
	return nil
}
