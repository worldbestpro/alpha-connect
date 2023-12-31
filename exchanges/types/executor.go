package types

import (
	"gitlab.com/alphaticks/alpha-connect/account"
	"gitlab.com/alphaticks/alpha-connect/config"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/utils"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	xchangerModels "gitlab.com/alphaticks/xchanger/models"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
	"gorm.io/gorm"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
)

type updateSecurityList struct{}
type updateMarketableProtocolAssetList struct{}

type ExecutorConfig struct {
	Exchanges               []*xchangerModels.Exchange
	Accounts                []*account.Account
	DialerPool              *xchangerUtils.DialerPool
	Registry                registry.StaticClient
	OpenseaCredentials      *xchangerModels.APICredentials
	DB                      *gorm.DB
	StrictExchange          bool
	ReadOnlyAccount         bool
	DisableAccountReconcile bool
	DisableAccountListener  bool
}

type Executor interface {
	actor.Actor
	OnSecurityListRequest(context actor.Context) error
	OnSecurityDefinitionRequest(context actor.Context) error
	OnMarketableProtocolAssetListRequest(context actor.Context) error
	OnHistoricalOpenInterestsRequest(context actor.Context) error
	OnHistoricalFundingRatesRequest(context actor.Context) error
	OnHistoricalLiquidationsRequest(context actor.Context) error
	OnMarketStatisticsRequest(context actor.Context) error
	OnMarketDataRequest(context actor.Context) error
	OnAccountInformationRequest(context actor.Context) error
	OnAccountMovementRequest(context actor.Context) error
	OnTradeCaptureReportRequest(context actor.Context) error
	OnOrderStatusRequest(context actor.Context) error
	OnPositionsRequest(context actor.Context) error
	OnBalancesRequest(context actor.Context) error
	OnNewOrderSingleRequest(context actor.Context) error
	OnNewOrderBulkRequest(context actor.Context) error
	OnOrderReplaceRequest(context actor.Context) error
	OnOrderBulkReplaceRequest(context actor.Context) error
	OnOrderCancelRequest(context actor.Context) error
	OnOrderMassCancelRequest(context actor.Context) error
	OnHistoricalUnipoolV3DataRequest(context actor.Context) error
	OnHistoricalSalesRequest(context actor.Context) error
	UpdateSecurityList(context actor.Context) error
	UpdateMarketableProtocolAssetList(context actor.Context) error
	GetLogger() *log.Logger
	Initialize(context actor.Context) error
	Clean(context actor.Context) error
}

type BaseExecutor struct {
	Config                   *config.Config
	DialerPool               *xchangerUtils.DialerPool
	Registry                 registry.StaticClient
	AccountClients           map[string]*http.Client
	SecuritiesLock           sync.RWMutex
	Securities               map[uint64]*models.Security
	MarketableProtocolAssets map[uint64]*models.MarketableProtocolAsset
	SymbolToSec              map[string]*models.Security
	HistoricalSecurities     map[uint64]*registry.Security
	SymbolToHistoricalSec    map[string]*registry.Security
}

func (state *BaseExecutor) GetSecurity(instr *models.Instrument) *models.Security {
	if instr == nil {
		return nil
	}
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()
	if instr.SecurityID != nil {
		if sec, ok := state.Securities[instr.SecurityID.Value]; ok {
			return sec
		}
	}
	if instr.Symbol != nil {
		if sec, ok := state.SymbolToSec[instr.Symbol.Value]; ok {
			return sec
		}
	}
	return nil
}

func ReceiveExecutor(state Executor, context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		if err := state.Initialize(context); err != nil {
			state.GetLogger().Error("error initializing", log.Error(err))
			panic(err)
		}
		state.GetLogger().Info("actor started")
		go func(pid *actor.PID) {
			time.Sleep(time.Minute)
			context.Send(pid, &updateSecurityList{})
		}(context.Self())

	case *actor.Stopping:
		if err := state.Clean(context); err != nil {
			state.GetLogger().Error("error stopping", log.Error(err))
			panic(err)
		}
		state.GetLogger().Info("actor stopping")

	case *actor.Stopped:
		state.GetLogger().Info("actor stopped")

	case *actor.Restarting:
		if err := state.Clean(context); err != nil {
			state.GetLogger().Error("error restarting", log.Error(err))
			// Attention, no panic in restarting or infinite loop
		}
		state.GetLogger().Info("actor restarting")

	case *utils.Ready:
		context.Respond(&utils.Ready{})

	case *messages.SecurityListRequest:
		if err := state.OnSecurityListRequest(context); err != nil {
			state.GetLogger().Error("error processing OnSecurityListRequest", log.Error(err))
			panic(err)
		}

	case *messages.SecurityDefinitionRequest:
		if err := state.OnSecurityDefinitionRequest(context); err != nil {
			state.GetLogger().Error("error processing OnSecurityDefinitionRequest", log.Error(err))
			panic(err)
		}

	case *messages.MarketableProtocolAssetListRequest:
		if err := state.OnMarketableProtocolAssetListRequest(context); err != nil {
			state.GetLogger().Error("error processing ProtocolAssetListRequest", log.Error(err))
			panic(err)
		}

	case *messages.HistoricalLiquidationsRequest:
		if err := state.OnHistoricalLiquidationsRequest(context); err != nil {
			state.GetLogger().Error("error processing OnHistoricalLiquidationRequest", log.Error(err))
			panic(err)
		}

	case *messages.HistoricalOpenInterestsRequest:
		if err := state.OnHistoricalOpenInterestsRequest(context); err != nil {
			state.GetLogger().Error("error processing OnHistoricalOpenInterestsRequest", log.Error(err))
			panic(err)
		}

	case *messages.HistoricalFundingRatesRequest:
		if err := state.OnHistoricalFundingRatesRequest(context); err != nil {
			state.GetLogger().Error("error processing OnHistoricalFundingRateRequest", log.Error(err))
			panic(err)
		}

	case *messages.MarketStatisticsRequest:
		if err := state.OnMarketStatisticsRequest(context); err != nil {
			state.GetLogger().Error("error processing OnMarketStatisticsRequest", log.Error(err))
			panic(err)
		}

	case *messages.MarketDataRequest:
		if err := state.OnMarketDataRequest(context); err != nil {
			state.GetLogger().Error("error processing MarketDataRequest", log.Error(err))
			panic(err)
		}

	case *messages.PositionsRequest:
		if err := state.OnPositionsRequest(context); err != nil {
			state.GetLogger().Error("error processing OnPositionListRequest", log.Error(err))
			panic(err)
		}

	case *messages.BalancesRequest:
		if err := state.OnBalancesRequest(context); err != nil {
			state.GetLogger().Error("error processing OnBalancesRequest", log.Error(err))
			panic(err)
		}

	case *messages.AccountInformationRequest:
		if err := state.OnAccountInformationRequest(context); err != nil {
			state.GetLogger().Error("error processing OnAccountInformationRequest", log.Error(err))
			panic(err)
		}

	case *messages.AccountMovementRequest:
		if err := state.OnAccountMovementRequest(context); err != nil {
			state.GetLogger().Error("error processing OnAccountMovementRequest", log.Error(err))
			panic(err)
		}

	case *messages.TradeCaptureReportRequest:
		if err := state.OnTradeCaptureReportRequest(context); err != nil {
			state.GetLogger().Error("error processing OnTradeCaptureReportRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderStatusRequest:
		if err := state.OnOrderStatusRequest(context); err != nil {
			state.GetLogger().Error("error processing OnOrderStatusRequest", log.Error(err))
			panic(err)
		}

	case *messages.NewOrderSingleRequest:
		if err := state.OnNewOrderSingleRequest(context); err != nil {
			state.GetLogger().Error("error processing OnNewSingleOrderRequest", log.Error(err))
			panic(err)
		}

	case *messages.NewOrderBulkRequest:
		if err := state.OnNewOrderBulkRequest(context); err != nil {
			state.GetLogger().Error("error processing OnNewOrderBulkRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderReplaceRequest:
		if err := state.OnOrderReplaceRequest(context); err != nil {
			state.GetLogger().Error("error processing OnOrderReplaceRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderBulkReplaceRequest:
		if err := state.OnOrderBulkReplaceRequest(context); err != nil {
			state.GetLogger().Error("erro procesing OnOrderBulkReplaceRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderCancelRequest:
		if err := state.OnOrderCancelRequest(context); err != nil {
			state.GetLogger().Error("error processing OnOrderCancelRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderMassCancelRequest:
		if err := state.OnOrderMassCancelRequest(context); err != nil {
			state.GetLogger().Error("error processing OnOrderMassCancelRequest", log.Error(err))
			panic(err)
		}

	case *messages.HistoricalUnipoolV3DataRequest:
		if err := state.OnHistoricalUnipoolV3DataRequest(context); err != nil {
			state.GetLogger().Error("error processing HistoricalUnipoolV3DataRequest", log.Error(err))
			panic(err)
		}

	case *messages.HistoricalSalesRequest:
		if err := state.OnHistoricalSalesRequest(context); err != nil {
			state.GetLogger().Error("error processing HistoricalSalesRequest", log.Error(err))
			panic(err)
		}

	case *updateSecurityList:
		go func() {
			if err := state.UpdateSecurityList(context); err != nil {
				state.GetLogger().Error("error fetching historical securities", log.Error(err))
			}
		}()
		go func(pid *actor.PID) {
			time.Sleep(time.Minute)
			context.Send(pid, &updateSecurityList{})
		}(context.Self())

	case *updateMarketableProtocolAssetList:
		go func() {
			if err := state.UpdateMarketableProtocolAssetList(context); err != nil {
				state.GetLogger().Info("error updating asset list", log.Error(err))
			}
		}()
		go func(pid *actor.PID) {
			time.Sleep(time.Minute)
			context.Send(pid, &updateMarketableProtocolAssetList{})
		}(context.Self())
	}
}

func (state *BaseExecutor) OnSecurityListRequest(context actor.Context) error {
	// Get http request and the expected response
	msg := context.Message().(*messages.SecurityListRequest)
	securities := make([]*models.Security, len(state.Securities))
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()

	i := 0
	for _, v := range state.Securities {
		securities[i] = v
		i += 1
	}
	context.Respond(&messages.SecurityList{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: securities})

	return nil
}

func (state *BaseExecutor) OnSecurityDefinitionRequest(context actor.Context) error {
	msg := context.Message().(*messages.SecurityDefinitionRequest)
	sec := state.GetSecurity(msg.Instrument)
	if sec != nil {
		context.Respond(&messages.SecurityDefinitionResponse{
			RequestID:  msg.RequestID,
			ResponseID: uint64(time.Now().UnixNano()),
			Success:    true,
			Security:   sec})
	} else {
		context.Respond(&messages.SecurityDefinitionResponse{
			RequestID:       msg.RequestID,
			ResponseID:      uint64(time.Now().UnixNano()),
			Success:         false,
			RejectionReason: messages.RejectionReason_UnknownSecurityID})
	}

	return nil
}

func (state *BaseExecutor) OnMarketableProtocolAssetListRequest(context actor.Context) error {
	// Get http request and the expected response
	msg := context.Message().(*messages.MarketableProtocolAssetListRequest)
	massets := make([]*models.MarketableProtocolAsset, len(state.MarketableProtocolAssets))
	i := 0
	for _, v := range state.MarketableProtocolAssets {
		massets[i] = v
		i += 1
	}
	context.Respond(&messages.MarketableProtocolAssetList{
		RequestID:                msg.RequestID,
		ResponseID:               uint64(time.Now().UnixNano()),
		Success:                  true,
		MarketableProtocolAssets: massets})

	return nil
}

func (state *BaseExecutor) OnHistoricalOpenInterestsRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalOpenInterestsRequest)
	context.Respond(&messages.HistoricalOpenInterestsResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnHistoricalFundingRatesRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalFundingRatesRequest)
	context.Respond(&messages.HistoricalFundingRatesResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnHistoricalLiquidationsRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalLiquidationsRequest)
	context.Respond(&messages.HistoricalLiquidationsResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnMarketStatisticsRequest(context actor.Context) error {
	req := context.Message().(*messages.MarketStatisticsRequest)
	context.Respond(&messages.MarketStatisticsResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnMarketDataRequest(context actor.Context) error {
	req := context.Message().(*messages.MarketDataRequest)
	context.Respond(&messages.MarketDataResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnAccountInformationRequest(context actor.Context) error {
	req := context.Message().(*messages.AccountInformationRequest)
	context.Respond(&messages.AccountInformationResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnAccountMovementRequest(context actor.Context) error {
	req := context.Message().(*messages.AccountMovementRequest)
	context.Respond(&messages.AccountMovementResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnTradeCaptureReportRequest(context actor.Context) error {
	req := context.Message().(*messages.TradeCaptureReportRequest)
	context.Respond(&messages.TradeCaptureReport{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnOrderStatusRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderStatusRequest)
	context.Respond(&messages.OrderList{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnPositionsRequest(context actor.Context) error {
	req := context.Message().(*messages.PositionsRequest)
	context.Respond(&messages.PositionList{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnBalancesRequest(context actor.Context) error {
	req := context.Message().(*messages.BalancesRequest)
	context.Respond(&messages.BalanceList{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnNewOrderSingleRequest(context actor.Context) error {
	req := context.Message().(*messages.NewOrderSingleRequest)
	context.Respond(&messages.NewOrderSingleResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnNewOrderBulkRequest(context actor.Context) error {
	req := context.Message().(*messages.NewOrderBulkRequest)
	context.Respond(&messages.NewOrderBulkResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnOrderReplaceRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderReplaceRequest)
	context.Respond(&messages.OrderReplaceResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnOrderBulkReplaceRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderBulkReplaceRequest)
	context.Respond(&messages.OrderBulkReplaceResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnOrderCancelRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderCancelRequest)
	context.Respond(&messages.OrderCancelResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnOrderMassCancelRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderMassCancelRequest)
	context.Respond(&messages.OrderMassCancelResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnHistoricalUnipoolV3DataRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalUnipoolV3DataRequest)
	context.Respond(&messages.HistoricalUnipoolV3DataResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) OnHistoricalSalesRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalSalesRequest)
	context.Respond(&messages.HistoricalSalesResponse{
		RequestID:       req.RequestID,
		ResponseID:      rand.Uint64(),
		Success:         false,
		RejectionReason: messages.RejectionReason_UnsupportedRequest,
	})
	return nil
}

func (state *BaseExecutor) UpdateSecurityList(context actor.Context) error {
	return nil
}

func (state *BaseExecutor) UpdateMarketableProtocolAssetList(context actor.Context) error {
	return nil
}

func (state *BaseExecutor) GetLogger() *log.Logger {
	return nil
}

func (state *BaseExecutor) Initialize(context actor.Context) error {
	return nil
}

func (state *BaseExecutor) Clean(context actor.Context) error {
	return nil
}

func (state *BaseExecutor) SyncSecurities(live []*models.Security, historical []*registry.Security) {
	state.SecuritiesLock.Lock()
	defer state.SecuritiesLock.Unlock()
	state.Securities = make(map[uint64]*models.Security)
	state.SymbolToSec = make(map[string]*models.Security)
	for _, s := range live {
		state.Securities[s.SecurityID] = s
		state.SymbolToSec[s.Symbol] = s
	}

	state.HistoricalSecurities = make(map[uint64]*registry.Security)
	state.SymbolToHistoricalSec = make(map[string]*registry.Security)
	for _, sec := range historical {
		state.HistoricalSecurities[sec.SecurityId] = sec
		state.SymbolToHistoricalSec[sec.Symbol] = sec
	}

	return
}

func (state *BaseExecutor) SymbolToSecurity(symbol string) *models.Security {
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()
	return state.SymbolToSec[symbol]
}

func (state *BaseExecutor) SymbolToHistoricalSecurity(symbol string) *registry.Security {
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()
	return state.SymbolToHistoricalSec[symbol]
}

func (state *BaseExecutor) IDToSecurity(ID uint64) *models.Security {
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()
	return state.Securities[ID]
}

func (state *BaseExecutor) IDToHistoricalSecurity(ID uint64) *registry.Security {
	state.SecuritiesLock.RLock()
	defer state.SecuritiesLock.RUnlock()
	return state.HistoricalSecurities[ID]
}

func (state *BaseExecutor) InstrumentToSymbol(instrument *models.Instrument) (string, *messages.RejectionReason) {
	if instrument == nil {
		v := messages.RejectionReason_MissingInstrument
		return "", &v
	}
	if instrument.Symbol != nil {
		sec := state.SymbolToSecurity(instrument.Symbol.Value)
		if sec == nil {
			hsec := state.SymbolToHistoricalSecurity(instrument.Symbol.Value)
			if hsec == nil {
				v := messages.RejectionReason_UnknownSymbol
				return "", &v
			} else {
				return hsec.Symbol, nil
			}
		} else {
			return sec.Symbol, nil
		}
	} else if instrument.SecurityID != nil {
		sec := state.IDToSecurity(instrument.SecurityID.Value)
		if sec == nil {
			hsec := state.IDToHistoricalSecurity(instrument.SecurityID.Value)
			if hsec == nil {
				v := messages.RejectionReason_UnknownSecurityID
				return "", &v
			} else {
				return hsec.Symbol, nil
			}
		}
		return sec.Symbol, nil
	} else {
		v := messages.RejectionReason_MissingInstrument
		return "", &v
	}
}

func (state *BaseExecutor) InstrumentToSecurity(instrument *models.Instrument) (*models.Security, *messages.RejectionReason) {
	if instrument == nil {
		v := messages.RejectionReason_MissingInstrument
		return nil, &v
	}
	if instrument.Symbol != nil {
		sec := state.SymbolToSecurity(instrument.Symbol.Value)
		if sec == nil {
			v := messages.RejectionReason_UnknownSymbol
			return nil, &v
		}
		return sec, nil
	} else if instrument.SecurityID != nil {
		sec := state.IDToSecurity(instrument.SecurityID.Value)
		if sec == nil {
			v := messages.RejectionReason_UnknownSecurityID
			return nil, &v
		}
		return sec, nil
	} else {
		v := messages.RejectionReason_MissingInstrument
		return nil, &v
	}
}
