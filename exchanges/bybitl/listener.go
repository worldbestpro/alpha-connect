package bybitl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	uuid "github.com/satori/go.uuid"
	"gitlab.com/alphaticks/alpha-connect/enum"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/gorderbook"
	gmodels "gitlab.com/alphaticks/gorderbook/gorderbook.models"
	"gitlab.com/alphaticks/xchanger"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges/bybitl"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math"
	"reflect"
	"sort"
	"time"
)

type checkSockets struct{}

type InstrumentData struct {
	orderBook           *gorderbook.OrderBookL2
	seqNum              uint64
	lastUpdateID        uint64
	lastUpdateTime      uint64
	lastHBTime          time.Time
	lastAggTradeTs      uint64
	lastLiquidationTime uint64
}

type Listener struct {
	ws             *bybitl.Websocket
	securityID     uint64
	security       *models.Security
	dialerPool     *xchangerUtils.DialerPool
	instrumentData *InstrumentData
	executor       *actor.PID
	logger         *log.Logger
	lastPingTime   time.Time
	socketTicker   *time.Ticker
}

func NewListenerProducer(securityID uint64, dialerPool *xchangerUtils.DialerPool) actor.Producer {
	return func() actor.Actor {
		return NewListener(securityID, dialerPool)
	}
}

func NewListener(securityID uint64, dialerPool *xchangerUtils.DialerPool) actor.Actor {
	return &Listener{
		securityID: securityID,
		dialerPool: dialerPool,
	}
}

func (state *Listener) Receive(context actor.Context) {
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

	case *messages.MarketDataRequest:
		if err := state.OnMarketDataRequest(context); err != nil {
			state.logger.Error("error processing OnMarketDataRequest", log.Error(err))
			panic(err)
		}

	case *xchanger.WebsocketMessage:
		if err := state.onWebsocketMessage(context); err != nil {
			state.logger.Error("error processing websocket message", log.Error(err))
			panic(err)
		}

	case *checkSockets:
		if err := state.checkSockets(context); err != nil {
			state.logger.Error("error checking socket", log.Error(err))
			panic(err)
		}
	}
}

func (state *Listener) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()),
		log.String("security-id", fmt.Sprintf("%d", state.securityID)))

	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.BYBITL.Name+"_executor")

	res, err := context.RequestFuture(state.executor, &messages.SecurityDefinitionRequest{
		RequestID:  0,
		Instrument: &models.Instrument{SecurityID: wrapperspb.UInt64(state.securityID)},
	}, 5*time.Second).Result()
	if err != nil {
		return fmt.Errorf("error fetching security definition: %v", err)
	}
	def := res.(*messages.SecurityDefinitionResponse)
	if !def.Success {
		return fmt.Errorf("error fetching security definition: %s", def.RejectionReason.String())
	}
	state.security = def.Security

	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()),
		log.String("security-id", fmt.Sprintf("%d", state.securityID)),
		log.String("exchange", state.security.Exchange.Name),
		log.String("symbol", state.security.Symbol))

	if state.security.MinPriceIncrement == nil || state.security.RoundLot == nil {
		return fmt.Errorf("security is missing MinPriceIncrement or RoundLot")
	}

	state.lastPingTime = time.Now()

	state.instrumentData = &InstrumentData{
		orderBook:           nil,
		seqNum:              uint64(time.Now().UnixNano()),
		lastUpdateID:        0,
		lastUpdateTime:      0,
		lastHBTime:          time.Now(),
		lastLiquidationTime: uint64(time.Now().UnixNano()) / 1000000,
	}

	if err := state.subscribeInstrument(context); err != nil {
		return fmt.Errorf("error subscribing to order book: %v", err)
	}

	socketTicker := time.NewTicker(5 * time.Second)
	state.socketTicker = socketTicker
	go func(pid *actor.PID) {
		for {
			select {
			case <-socketTicker.C:
				context.Send(pid, &checkSockets{})
			case <-time.After(10 * time.Second):
				if state.socketTicker != socketTicker {
					// Only stop if socket ticker has changed
					return
				}
			}
		}
	}(context.Self())

	return nil
}

func (state *Listener) Clean(context actor.Context) error {
	if state.ws != nil {
		if err := state.ws.Disconnect(); err != nil {
			state.logger.Info("error disconnecting socket", log.Error(err))
		}
	}

	if state.socketTicker != nil {
		state.socketTicker.Stop()
		state.socketTicker = nil
	}

	return nil
}

func (state *Listener) subscribeInstrument(context actor.Context) error {
	if state.ws != nil {
		_ = state.ws.Disconnect()
	}

	ws := bybitl.NewWebsocket()
	if err := ws.ConnectPublic(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to bybit websocket: %v", err)
	}

	if err := ws.SubscribeOrderBook(state.security.Symbol, bybitl.WSOrderBookDepth200, bybitl.WS100ms); err != nil {
		return fmt.Errorf("error subscribing to orderbook for %s", state.security.Symbol)
	}

	var ob *gorderbook.OrderBookL2
	nTries := 0
	for nTries < 100 {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}

		switch ws.Msg.Message.(type) {
		case bybitl.OrderBookSnapshot:
			res := ws.Msg.Message.(bybitl.OrderBookSnapshot)
			bids, asks := res.ToBidAsk()
			tickPrecision := uint64(math.Ceil(1. / state.security.MinPriceIncrement.Value))
			lotPrecision := uint64(math.Ceil(1. / state.security.RoundLot.Value))
			ob = gorderbook.NewOrderBookL2(
				tickPrecision,
				lotPrecision,
				10000)
			ob.Sync(bids, asks)
			if ob.Crossed() {
				return fmt.Errorf("crossed orderbook")
			}
			ts := uint64(ws.Msg.ClientTime.UnixNano()) / 1000000
			state.instrumentData.orderBook = ob
			state.instrumentData.lastUpdateTime = ts
			state.instrumentData.lastUpdateID = res.Sequence
			state.instrumentData.seqNum = uint64(time.Now().UnixNano())
			nTries = 100

		case bybitl.WSResponse:
			res := ws.Msg.Message.(bybitl.WSResponse)
			if !res.Success {
				err := fmt.Errorf("error getting orderbook: %s", res.ReturnMessage)
				return err
			}
		}
		nTries += 1
	}

	if ob == nil {
		return fmt.Errorf("error getting orderbook")
	}

	if err := ws.SubscribeTrade(state.security.Symbol); err != nil {
		return fmt.Errorf("error subscribing to trades for %s", state.security.Symbol)
	}

	if err := ws.SubscribeInstrumentInfo(state.security.Symbol, "100ms"); err != nil {
		return fmt.Errorf("error subscribing to instrument info for %s", state.security.Symbol)
	}

	if err := ws.SubscribeLiquidation(state.security.Symbol); err != nil {
		return fmt.Errorf("error subscribing to liquidation for %s", state.security.Symbol)
	}

	state.ws = ws

	go func(ws *bybitl.Websocket, pid *actor.PID) {
		for ws.ReadMessage() {
			context.Send(pid, ws.Msg)
		}
	}(ws, context.Self())

	return nil
}

func (state *Listener) OnMarketDataRequest(context actor.Context) error {
	msg := context.Message().(*messages.MarketDataRequest)
	response := &messages.MarketDataResponse{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		SeqNum:     state.instrumentData.seqNum,
		Success:    true,
	}
	if msg.Aggregation == models.OrderBookAggregation_L2 {
		snapshot := &models.OBL2Snapshot{
			Bids:          state.instrumentData.orderBook.GetBids(0),
			Asks:          state.instrumentData.orderBook.GetAsks(0),
			Timestamp:     utils.MilliToTimestamp(state.instrumentData.lastUpdateTime),
			TickPrecision: &wrapperspb.UInt64Value{Value: state.instrumentData.orderBook.TickPrecision},
			LotPrecision:  &wrapperspb.UInt64Value{Value: state.instrumentData.orderBook.LotPrecision},
		}
		response.SnapshotL2 = snapshot
	}

	context.Respond(response)
	return nil
}

func (state *Listener) onWebsocketMessage(context actor.Context) error {
	msg := context.Message().(*xchanger.WebsocketMessage)
	switch msg.Message.(type) {

	case error:
		return fmt.Errorf("OB socket error: %v", msg)

	case bybitl.OrderBookDelta:
		if state.ws == nil || msg.WSID != state.ws.ID {
			return nil
		}

		ts := uint64(msg.ClientTime.UnixNano() / 1000000)
		update := msg.Message.(bybitl.OrderBookDelta)

		instr := state.instrumentData

		obDelta := &models.OBL2Update{
			Levels:    nil,
			Timestamp: utils.MilliToTimestamp(ts),
			Trade:     false,
		}

		for _, l := range update.Insert {
			level := &gmodels.OrderBookLevel{
				Price:    l.Price,
				Quantity: l.Size,
				Bid:      l.Side == "Buy",
			}
			instr.orderBook.UpdateOrderBookLevel(level)
			obDelta.Levels = append(obDelta.Levels, level)
		}
		for _, l := range update.Update {
			level := &gmodels.OrderBookLevel{
				Price:    l.Price,
				Quantity: l.Size,
				Bid:      l.Side == "Buy",
			}
			instr.orderBook.UpdateOrderBookLevel(level)
			obDelta.Levels = append(obDelta.Levels, level)
		}
		for _, l := range update.Delete {
			level := &gmodels.OrderBookLevel{
				Price:    l.Price,
				Quantity: 0.,
				Bid:      l.Side == "Buy",
			}
			instr.orderBook.UpdateOrderBookLevel(level)
			obDelta.Levels = append(obDelta.Levels, level)
		}

		if state.instrumentData.orderBook.Crossed() {
			state.logger.Info("crossed orderbook", log.Error(errors.New("crossed")))
			return state.subscribeInstrument(context)
		}

		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			UpdateL2: obDelta,
			SeqNum:   state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1

		instr.lastUpdateTime = ts
		instr.lastUpdateID = update.Sequence
		//state.postSnapshot(context)

	case bybitl.TradeDelta:
		ts := uint64(msg.ClientTime.UnixNano() / 1000000)
		trades := msg.Message.(bybitl.TradeDelta)
		if len(trades) == 0 {
			break
		}

		sort.Slice(trades, func(i, j int) bool {
			return trades[i].TradeTime < trades[j].TradeTime
		})

		var aggTrade *models.AggregatedTrade
		var aggHelpR uint64 = 0
		for _, trade := range trades {
			aggHelp := trade.TradeTime * 10
			// do that so new agg trade if side changes
			if trade.Side == "Sell" {
				aggHelp += 1
			}
			tradeID, err := uuid.FromString(trade.TradeID)
			if err != nil {
				return fmt.Errorf("error parsing trade ID: %v", err)
			}
			if aggTrade == nil || aggHelpR != aggHelp {

				if aggTrade != nil {
					// Send aggregate trade
					context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
						Trades: []*models.AggregatedTrade{aggTrade},
						SeqNum: state.instrumentData.seqNum + 1,
					})
					state.instrumentData.seqNum += 1
					state.instrumentData.lastAggTradeTs = ts
				}

				if ts <= state.instrumentData.lastAggTradeTs {
					ts = state.instrumentData.lastAggTradeTs + 1
				}
				aggTrade = &models.AggregatedTrade{
					Bid:         trade.Side == "Sell",
					Timestamp:   utils.MilliToTimestamp(ts),
					AggregateID: binary.LittleEndian.Uint64(tradeID.Bytes()[0:8]),
					Trades:      nil,
				}
				aggHelpR = aggHelp
			}

			trd := &models.Trade{
				Price:    trade.Price,
				Quantity: trade.Size,
				ID:       binary.LittleEndian.Uint64(tradeID.Bytes()[0:8]),
			}

			aggTrade.Trades = append(aggTrade.Trades, trd)
		}
		if aggTrade != nil {
			// Send aggregate trade
			context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
				Trades: []*models.AggregatedTrade{aggTrade},
				SeqNum: state.instrumentData.seqNum + 1,
			})
			state.instrumentData.seqNum += 1
			state.instrumentData.lastAggTradeTs = ts
		}

	case bybitl.WSLiquidation:
		l := msg.Message.(bybitl.WSLiquidation)
		liq := &models.Liquidation{
			Bid:       l.Side == "Sell",
			Timestamp: utils.MilliToTimestamp(l.Time),
			OrderID:   0,
			Price:     l.Price,
			Quantity:  l.Quantity,
		}
		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			Liquidation: liq,
			SeqNum:      state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		state.instrumentData.lastLiquidationTime = utils.TimestampToMilli(liq.Timestamp)

	case bybitl.InstrumentInfoSnapshot:
		info := msg.Message.(bybitl.InstrumentInfoSnapshot)
		ts := uint64(msg.ClientTime.UnixNano() / 1000000)
		refresh := &messages.MarketDataIncrementalRefresh{
			Stats: []*models.Stat{{
				Timestamp: utils.MilliToTimestamp(ts),
				StatType:  models.StatType_OpenInterest,
				Value:     float64(info.OpenInterestE8) / 100000000,
			}},
			SeqNum: state.instrumentData.seqNum + 1,
		}
		if state.security.SecurityType == enum.SecurityType_CRYPTO_PERP {
			refresh.Stats = append(refresh.Stats, &models.Stat{
				Timestamp: utils.MilliToTimestamp(uint64(info.NextFundingTime.UnixNano() / 1000000)),
				StatType:  models.StatType_FundingRate,
				Value:     float64(info.FundingRateE6) / 1000000,
			})
		}
		context.Send(context.Parent(), refresh)
		state.instrumentData.seqNum += 1

	case bybitl.InstrumentInfoDelta:
		delta := msg.Message.(bybitl.InstrumentInfoDelta)
		ts := uint64(msg.ClientTime.UnixNano() / 1000000)
		refresh := &messages.MarketDataIncrementalRefresh{
			SeqNum: state.instrumentData.seqNum + 1,
		}
		for _, info := range delta.Update {
			if info.OpenInterestE8 != nil {
				refresh.Stats = append(refresh.Stats, &models.Stat{
					Timestamp: utils.MilliToTimestamp(ts),
					StatType:  models.StatType_OpenInterest,
					Value:     float64(*info.OpenInterestE8) / 100000000,
				})
			}
			if state.security.SecurityType == enum.SecurityType_CRYPTO_PERP && info.FundingRateE6 != nil && info.NextFundingTime != nil {
				refresh.Stats = append(refresh.Stats, &models.Stat{
					Timestamp: utils.MilliToTimestamp(uint64(info.NextFundingTime.UnixNano() / 1000000)),
					StatType:  models.StatType_FundingRate,
					Value:     float64(*info.FundingRateE6) / 1000000,
				})
			}
		}
		context.Send(context.Parent(), refresh)
		state.instrumentData.seqNum += 1

	case bybitl.WSResponse:

	default:
		state.logger.Info("received unknown message",
			log.String("message_type",
				reflect.TypeOf(msg.Message).String()))
	}

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {
	// If haven't sent anything for 2 seconds, send heartbeat
	if time.Since(state.instrumentData.lastHBTime) > 2*time.Second {
		// Send an empty refresh
		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			SeqNum: state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		state.instrumentData.lastHBTime = time.Now()
	}

	if state.ws.Err != nil || !state.ws.Connected {
		if state.ws.Err != nil {
			state.logger.Info("error on socket", log.Error(state.ws.Err))
		}
		if err := state.subscribeInstrument(context); err != nil {
			return fmt.Errorf("error subscribing to instrument: %v", err)
		}
	}

	return nil
}
