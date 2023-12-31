package bitmex

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
	"gitlab.com/alphaticks/xchanger/exchanges/bitmex"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math"
	"reflect"
	"time"
)

type checkSockets struct{}

type InstrumentData struct {
	orderBook      *gorderbook.OrderBookL2
	seqNum         uint64
	lastUpdateTime uint64
	lastHBTime     time.Time
	levelIDToPrice map[int64]float64
	lastAggTradeTs uint64
}

// OBType: OBL3
// No ID for the deltas..

type Listener struct {
	ws             *bitmex.Websocket
	securityID     uint64
	security       *models.Security
	dialerPool     *xchangerUtils.DialerPool
	executor       *actor.PID
	instrumentData *InstrumentData
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
	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.BITMEX.Name+"_executor")

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

	state.instrumentData = &InstrumentData{
		orderBook:      nil,
		seqNum:         uint64(time.Now().UnixNano()),
		lastUpdateTime: 0,
		lastHBTime:     time.Now(),
		levelIDToPrice: make(map[int64]float64),
		lastAggTradeTs: 0,
	}

	if err := state.subscribeInstrument(context); err != nil {
		return fmt.Errorf("error subscribing to instrument: %v", err)
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

	ws := bitmex.NewWebsocket()
	if err := ws.Connect(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to bitmex websocket: %v", err)
	}

	if err := ws.SubscribeSymbol(state.security.Symbol, bitmex.WSOrderBookL2StreamName); err != nil {
		return fmt.Errorf("error subscribing to OBL2 stream: %v", err)
	}

	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}
	resp, ok := ws.Msg.Message.(bitmex.WSSubscribeResponse)
	if !ok {
		return fmt.Errorf("was expecting WSSubscribeResponse, got %s", reflect.TypeOf(ws.Msg.Message).String())
	}
	if !resp.Success {
		return fmt.Errorf("error subscribing to orderbook: %s", resp.Subscribe)
	}

	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}
	obData, ok := ws.Msg.Message.(bitmex.WSOrderBookData)
	if !ok {
		return fmt.Errorf("was expecting depth data, got %s", reflect.TypeOf(ws.Msg.Message).String())
	}

	if obData.Action == "partial" || obData.Action == "insert" {
		for _, data := range obData.Data {
			state.instrumentData.levelIDToPrice[data.ID] = data.Price
		}
	}

	var bids, asks []*gmodels.OrderBookLevel

	for _, data := range obData.Data {
		if data.Side == "Buy" {
			bids = append(bids, &gmodels.OrderBookLevel{
				Price:    state.instrumentData.levelIDToPrice[data.ID],
				Quantity: float64(data.Size),
				Bid:      true,
			})
		} else {
			asks = append(asks, &gmodels.OrderBookLevel{
				Price:    state.instrumentData.levelIDToPrice[data.ID],
				Quantity: float64(data.Size),
				Bid:      false,
			})
		}
	}
	ts := uint64(ws.Msg.ClientTime.UnixNano()) / 1000000
	// TODO depth
	tickPrecision := uint64(math.Ceil(1. / state.security.MinPriceIncrement.Value))
	lotPrecision := uint64(math.Ceil(1. / state.security.RoundLot.Value))

	ob := gorderbook.NewOrderBookL2(
		tickPrecision,
		lotPrecision,
		10000)

	ob.Sync(bids, asks)
	if ob.Crossed() {
		return fmt.Errorf("crossed orderbook")
	}
	state.instrumentData.seqNum = uint64(time.Now().UnixNano())
	state.instrumentData.orderBook = ob
	state.instrumentData.lastUpdateTime = ts

	if err := ws.SubscribeSymbol(state.security.Symbol, bitmex.WSTradeStreamName); err != nil {
		return fmt.Errorf("error subscribing to trade stream: %v", err)
	}
	if err := ws.SubscribeSymbol(state.security.Symbol, bitmex.WSLiquidationStreamName); err != nil {
		return fmt.Errorf("error subscribing to liquidation stream: %v", err)
	}
	if err := ws.SubscribeSymbol(state.security.Symbol, bitmex.WSInstrumentStreamName); err != nil {
		return fmt.Errorf("error subscribing to instrument stream: %v", err)
	}

	if state.security.SecurityType == enum.SecurityType_CRYPTO_PERP {
		if err := ws.SubscribeSymbol(state.security.Symbol, bitmex.WSFundingStreamName); err != nil {
			return fmt.Errorf("error subscribing to funding stream: %v", err)
		}
	}
	state.ws = ws

	go func(ws *bitmex.Websocket, pid *actor.PID) {
		for ws.ReadMessage() {
			context.Send(pid, ws.Msg)
		}
	}(ws, context.Self())

	return nil
}

func (state *Listener) OnMarketDataRequest(context actor.Context) error {
	msg := context.Message().(*messages.MarketDataRequest)

	snapshot := &models.OBL2Snapshot{
		Bids:          state.instrumentData.orderBook.GetBids(0),
		Asks:          state.instrumentData.orderBook.GetAsks(0),
		Timestamp:     utils.MilliToTimestamp(state.instrumentData.lastUpdateTime),
		TickPrecision: &wrapperspb.UInt64Value{Value: state.instrumentData.orderBook.TickPrecision},
		LotPrecision:  &wrapperspb.UInt64Value{Value: state.instrumentData.orderBook.LotPrecision},
	}
	context.Respond(&messages.MarketDataResponse{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		SnapshotL2: snapshot,
		SeqNum:     state.instrumentData.seqNum,
		Success:    true,
	})

	return nil
}

func (state *Listener) onWebsocketMessage(context actor.Context) error {
	msg := context.Message().(*xchanger.WebsocketMessage)
	if state.ws == nil || msg.WSID != state.ws.ID {
		return nil
	}
	switch msg.Message.(type) {

	case error:
		return fmt.Errorf("socket error: %v", msg)

	case bitmex.WSErrorResponse:
		err := msg.Message.(bitmex.WSErrorResponse)
		return fmt.Errorf("socket error: %v", err)

	case bitmex.WSOrderBookData:
		obData := msg.Message.(bitmex.WSOrderBookData)

		if obData.Action == "partial" || obData.Action == "insert" {
			for _, data := range obData.Data {
				state.instrumentData.levelIDToPrice[data.ID] = data.Price
			}
		}

		var bids, asks []*gmodels.OrderBookLevel

		for _, data := range obData.Data {
			if data.Side == "Buy" {
				bids = append(bids, &gmodels.OrderBookLevel{
					Price:    state.instrumentData.levelIDToPrice[data.ID],
					Quantity: float64(data.Size),
					Bid:      true,
				})
			} else {
				asks = append(asks, &gmodels.OrderBookLevel{
					Price:    state.instrumentData.levelIDToPrice[data.ID],
					Quantity: float64(data.Size),
					Bid:      false,
				})
			}
		}
		ts := uint64(msg.ClientTime.UnixNano()) / 1000000

		limitDelta := &models.OBL2Update{
			Levels:    []*gmodels.OrderBookLevel{},
			Timestamp: utils.MilliToTimestamp(ts),
			Trade:     false,
		}

		for _, bid := range bids {
			state.instrumentData.orderBook.UpdateOrderBookLevel(bid)
			limitDelta.Levels = append(limitDelta.Levels, bid)
		}
		for _, ask := range asks {
			state.instrumentData.orderBook.UpdateOrderBookLevel(ask)
			limitDelta.Levels = append(limitDelta.Levels, ask)
		}

		if state.instrumentData.orderBook.Crossed() {
			state.logger.Info("crossed orderbook", log.Error(errors.New("crossed")))
			return state.subscribeInstrument(context)
		}

		state.instrumentData.lastUpdateTime = ts

		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			UpdateL2: limitDelta,
			SeqNum:   state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1

	case bitmex.WSTradeData:
		tradeData := msg.Message.(bitmex.WSTradeData)
		var aggTrade *models.AggregatedTrade
		ts := uint64(msg.ClientTime.UnixNano()) / 1000000
		for _, trade := range tradeData.Data {
			aggID := (uint64(trade.Timestamp.UnixNano()) / 1000) * 10
			// Add one to aggregatedID if it's a sell so that
			// buy and sell happening at the same time won't have the same ID
			if trade.Side == "Sell" {
				aggID += 1
			}
			tradeID, err := uuid.FromString(trade.TrdMatchID)
			if err != nil {
				return fmt.Errorf("error parsing trade ID: %v", err)
			}

			if aggTrade == nil || aggTrade.AggregateID != aggID {

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
					AggregateID: aggID,
					Trades:      nil,
				}
			}
			trade := &models.Trade{
				Price:    trade.Price,
				Quantity: float64(trade.Size),
				ID:       binary.LittleEndian.Uint64(tradeID.Bytes()[0:8]),
			}
			aggTrade.Trades = append(aggTrade.Trades, trade)
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

	case bitmex.WSLiquidationData:
		liqData := msg.Message.(bitmex.WSLiquidationData)
		ts := uint64(msg.ClientTime.UnixNano()) / 1000000
		for _, d := range liqData.Data {
			orderID, err := uuid.FromString(d.OrderID)
			if err != nil {
				return fmt.Errorf("error parsing trade ID: %v", err)
			}
			context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
				Liquidation: &models.Liquidation{
					Bid:       d.Side == "Buy",
					Timestamp: utils.MilliToTimestamp(ts),
					OrderID:   binary.LittleEndian.Uint64(orderID.Bytes()[0:8]),
					Price:     d.Price,
					Quantity:  float64(d.LeavesQty),
				},
				SeqNum: state.instrumentData.seqNum + 1,
			})
			state.instrumentData.seqNum += 1
		}

	case bitmex.WSFundingData:
		fundData := msg.Message.(bitmex.WSFundingData)
		for _, f := range fundData.Data {
			refresh := &messages.MarketDataIncrementalRefresh{
				SeqNum: state.instrumentData.seqNum + 1,
			}
			refresh.Stats = append(refresh.Stats, &models.Stat{
				Timestamp: utils.MilliToTimestamp(uint64(f.Timestamp.UnixNano() / 1000000)),
				StatType:  models.StatType_FundingRate,
				Value:     f.FundingRate,
			})
			context.Send(context.Parent(), refresh)
			state.instrumentData.seqNum += 1
		}

	case bitmex.WSInstrumentData:
		instrData := msg.Message.(bitmex.WSInstrumentData)
		for _, f := range instrData.Data {
			if f.OpenInterest != nil {
				context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
					Stats: []*models.Stat{{
						Timestamp: utils.MilliToTimestamp(uint64(f.Timestamp.UnixNano() / 1000000)),
						StatType:  models.StatType_OpenInterest,
						Value:     float64(*f.OpenInterest), //TODO / 1e4,
					}},
					SeqNum: state.instrumentData.seqNum + 1,
				})
				state.instrumentData.seqNum += 1
			}
		}
	}

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {

	if time.Since(state.lastPingTime) > 10*time.Second {
		_ = state.ws.Ping()

		state.lastPingTime = time.Now()
	}

	if state.ws.Err != nil || !state.ws.Connected {
		if state.ws.Err != nil {
			state.logger.Info("error on socket", log.Error(state.ws.Err))
		}
		if err := state.subscribeInstrument(context); err != nil {
			return fmt.Errorf("error subscribing to instrument: %v", err)
		}
	}

	// If haven't sent anything for 2 seconds, send heartbeat
	if time.Since(state.instrumentData.lastHBTime) > 2*time.Second {
		// Send an empty refresh
		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			SeqNum: state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		state.instrumentData.lastHBTime = time.Now()
	}

	return nil
}
