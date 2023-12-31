package chains

import (
	"gitlab.com/alphaticks/alpha-connect/config"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	"reflect"
	"time"

	models2 "gitlab.com/alphaticks/xchanger/models"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
)

// The executor routes all the request to the underlying exchange executor & listeners

type Executor struct {
	cfg       *config.Config
	registry  registry.StaticClient
	executors map[uint32]*actor.PID // A map from exchange ID to executor
	logger    *log.Logger
	strict    bool
}

func NewExecutorProducer(cfg *config.Config, registry registry.StaticClient) actor.Producer {
	return func() actor.Actor {
		return NewExecutor(cfg, registry)
	}
}

func NewExecutor(cfg *config.Config, registry registry.StaticClient) actor.Actor {
	return &Executor{
		cfg:      cfg,
		registry: registry,
	}
}

func (state *Executor) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		if err := state.Initialize(context); err != nil {
			state.logger.Error("error initializing", log.Error(err))
			panic(err)
		}
		state.logger.Info("actor started")
	case *actor.Stopping:
		if err := state.Clean(context); err != nil {
			state.logger.Error("error stopping", log.Error(err))
			panic(err)
		}
		state.logger.Info("actor stopping")
	case *actor.Stopped:
		state.logger.Info("actor stopped")
	case *actor.Restarting:
		if err := state.Clean(context); err != nil {
			state.logger.Error("error restarting", log.Error(err))
			// Attention, no panic in restarting or infinite loop
		}
		state.logger.Info("actor restarting")
	case *messages.BlockNumberRequest:
		if err := state.OnBlockNumberRequest(context); err != nil {
			state.logger.Error("error processing OnBlockNumberRequest", log.Error(err))
			panic(err)
		}
	case *messages.BlockInfoRequest:
		if err := state.OnBlockInfoRequest(context); err != nil {
			state.logger.Error("error processing OnBlockInfoRequest", log.Error(err))
			panic(err)
		}
	case *messages.EVMLogsQueryRequest:
		if err := state.OnEVMLogsQueryRequest(context); err != nil {
			state.logger.Error("error processing OnEVMLogsQueryRequest", log.Error(err))
			panic(err)
		}
	case *messages.EVMLogsSubscribeRequest:
		if err := state.OnEVMLogsSubscribeRequest(context); err != nil {
			state.logger.Error("error processing OnEVMLogsSubscribeRequest", log.Error(err))
			panic(err)
		}
	case *messages.EVMContractCallRequest:
		if err := state.OnEVMContractCallRequest(context); err != nil {
			state.logger.Error("error processing OnEVMContractCallRequest", log.Error(err))
			panic(err)
		}
	case *messages.SVMEventsQueryRequest:
		if err := state.OnSVMEventsQueryRequest(context); err != nil {
			state.logger.Error("error processing OnSVMEventsQueryRequest", log.Error(err))
			panic(err)
		}
	case *messages.SVMContractCallRequest:
		if err := state.OnSVMContractCallRequest(context); err != nil {
			state.logger.Error("error processing OnSVMContractCallRequest", log.Error(err))
			panic(err)
		}
	case *messages.SVMContractClassRequest:
		if err := state.OnSVMContractClassRequest(context); err != nil {
			state.logger.Error("error processing OnSVMContractClassRequest", log.Error(err))
			panic(err)
		}
	case *messages.SVMBlockQueryRequest:
		if err := state.OnSVMBlockQueryRequest(context); err != nil {
			state.logger.Error("error processing OnSVMBlockQueryRequest", log.Error(err))
			panic(err)
		}
	case *messages.SVMTransactionByHashRequest:
		if err := state.OnSVMTransactionByHashRequest(context); err != nil {
			state.logger.Error("error processing OnSVMTransactionByHashRequest", log.Error(err))
			panic(err)
		}
	}
}

func (state *Executor) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))

	state.executors = make(map[uint32]*actor.PID)
	return nil
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) OnBlockNumberRequest(context actor.Context) error {
	req := context.Message().(*messages.BlockNumberRequest)
	if req.Chain == nil {
		context.Respond(&messages.BlockNumberResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.BlockNumberResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnBlockInfoRequest(context actor.Context) error {
	req := context.Message().(*messages.BlockInfoRequest)
	if req.Chain == nil {
		context.Respond(&messages.BlockInfoResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.BlockInfoResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnEVMLogsQueryRequest(context actor.Context) error {
	req := context.Message().(*messages.EVMLogsQueryRequest)
	if req.Chain == nil {
		context.Respond(&messages.EVMLogsQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.EVMLogsQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnEVMLogsSubscribeRequest(context actor.Context) error {
	req := context.Message().(*messages.EVMLogsSubscribeRequest)
	if req.Chain == nil {
		context.Respond(&messages.EVMLogsSubscribeResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}
	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.EVMLogsSubscribeResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnEVMContractCallRequest(context actor.Context) error {
	req := context.Message().(*messages.EVMContractCallRequest)
	if req.Chain == nil {
		context.Respond(&messages.EVMContractCallResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}
	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.EVMContractCallResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnSVMEventsQueryRequest(context actor.Context) error {
	req := context.Message().(*messages.SVMEventsQueryRequest)
	if req.Chain == nil {
		context.Respond(&messages.SVMEventsQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.SVMEventsQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnSVMContractCallRequest(context actor.Context) error {
	req := context.Message().(*messages.SVMContractCallRequest)
	if req.Chain == nil {
		context.Respond(&messages.SVMContractCallResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.SVMContractCallResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnSVMContractClassRequest(context actor.Context) error {
	req := context.Message().(*messages.SVMContractClassRequest)
	if req.Chain == nil {
		context.Respond(&messages.SVMContractClassResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.SVMContractClassResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnSVMBlockQueryRequest(context actor.Context) error {
	req := context.Message().(*messages.SVMBlockQueryRequest)
	if req.Chain == nil {
		context.Respond(&messages.SVMBlockQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.SVMBlockQueryResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) OnSVMTransactionByHashRequest(context actor.Context) error {
	req := context.Message().(*messages.SVMTransactionByHashRequest)
	if req.Chain == nil {
		context.Respond(&messages.SVMTransactionByHashResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownChain,
		})
		return nil
	}

	if rej := state.forward(context, req.Chain); rej != nil {
		context.Respond(&messages.SVMTransactionByHashResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *rej,
		})
		return nil
	}
	return nil
}

func (state *Executor) forward(context actor.Context, chain *models2.Chain) *messages.RejectionReason {
	pid, ok := state.executors[chain.ID]
	if !ok {
		// Find the rpc
		rpc := ""
		for _, r := range state.cfg.ChainRPCs {
			if r.Chain == chain.ID {
				rpc = r.Endpoint
			}
		}
		if rpc == "" {
			tmp := messages.RejectionReason_UnknownChain
			return &tmp
		}
		producer := NewChainExecutorProducer(chain, rpc, state.registry)
		if producer == nil {
			tmp := messages.RejectionReason_UnknownChain
			return &tmp
		}
		props := actor.PropsFromProducer(producer, actor.WithSupervisor(
			actor.NewExponentialBackoffStrategy(100*time.Second, time.Second)))
		pid = context.Spawn(props)
		state.executors[chain.ID] = pid
	}
	context.Forward(pid)
	return nil
}
