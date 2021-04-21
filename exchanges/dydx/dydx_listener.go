package dydx

import (
	"errors"
	"fmt"
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/log"
	"github.com/gogo/protobuf/types"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/gorderbook"
	"gitlab.com/alphaticks/xchanger/exchanges/dydx"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
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
	levelOffset    map[uint64]uint64
	lastAggTradeTs uint64
}

// OBType: OBL3
// No ID for the deltas..

type Listener struct {
	ws              *dydx.Websocket
	security        *models.Security
	dialerPool      *xchangerUtils.DialerPool
	instrumentData  *InstrumentData
	executorManager *actor.PID
	logger          *log.Logger
	lastPingTime    time.Time
	socketTicker    *time.Ticker
}

func NewListenerProducer(security *models.Security, dialerPool *xchangerUtils.DialerPool) actor.Producer {
	return func() actor.Actor {
		return NewListener(security, dialerPool)
	}
}

func NewListener(security *models.Security, dialerPool *xchangerUtils.DialerPool) actor.Actor {
	return &Listener{
		ws:              nil,
		security:        security,
		dialerPool:      dialerPool,
		instrumentData:  nil,
		executorManager: nil,
		logger:          nil,
		socketTicker:    nil,
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

	case *dydx.WebsocketMessage:
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
		log.String("exchange", state.security.Exchange.Name),
		log.String("symbol", state.security.Symbol))

	if state.security.MinPriceIncrement == nil || state.security.RoundLot == nil {
		return fmt.Errorf("security is missing MinPriceIncrement or RoundLot")
	}
	state.executorManager = actor.NewPID(context.ActorSystem().Address(), "exchange_executor_manager")

	state.instrumentData = &InstrumentData{
		orderBook:      nil,
		seqNum:         uint64(time.Now().UnixNano()),
		lastUpdateTime: 0,
		lastHBTime:     time.Now(),
		levelOffset:    make(map[uint64]uint64),
		lastAggTradeTs: 0,
	}

	if err := state.subscribeInstrument(context); err != nil {
		return fmt.Errorf("error subscribing to order book: %v", err)
	}

	socketTicker := time.NewTicker(5 * time.Second)
	state.socketTicker = socketTicker
	go func(pid *actor.PID) {
		for {
			select {
			case _ = <-socketTicker.C:
				context.Send(pid, &checkSockets{})
			case <-time.After(10 * time.Second):
				// timer stopped, we leave
				return
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

	ws := dydx.NewWebsocket()
	if err := ws.Connect(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to bitmex websocket: %v", err)
	}
	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}

	if err := ws.Subscribe(state.security.Symbol, dydx.WSOrderBookV3); err != nil {
		return fmt.Errorf("error subscribing to OBL2 stream: %v", err)
	}

	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}
	resp, ok := ws.Msg.Message.(dydx.WSOrderBookSubscribed)
	if !ok {
		return fmt.Errorf("was expecting WSOrderBookSubscribed, got %s", reflect.TypeOf(ws.Msg.Message).String())
	}
	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}

	tickPrecision := uint64(math.Ceil(1. / state.security.MinPriceIncrement.Value))
	lotPrecision := uint64(math.Ceil(1. / state.security.RoundLot.Value))

	var bids, asks []gorderbook.OrderBookLevel
	for _, l := range resp.Contents.Bids {
		if uint64(math.Round(l.Size*float64(lotPrecision))) == 0 {
			continue
		}
		bids = append(bids, gorderbook.OrderBookLevel{
			Price:    l.Price,
			Quantity: l.Size,
			Bid:      true,
		})
		k := uint64(math.Round(l.Price * float64(tickPrecision)))
		state.instrumentData.levelOffset[k] = resp.Contents.Offset
	}
	for _, l := range resp.Contents.Asks {
		if uint64(math.Round(l.Size*float64(lotPrecision))) == 0 {
			continue
		}
		asks = append(asks, gorderbook.OrderBookLevel{
			Price:    l.Price,
			Quantity: l.Size,
			Bid:      false,
		})
		k := uint64(math.Round(l.Price * float64(tickPrecision)))
		state.instrumentData.levelOffset[k] = resp.Contents.Offset
	}
	ts := uint64(ws.Msg.Time.UnixNano()) / 1000000
	// TODO depth

	ob := gorderbook.NewOrderBookL2(
		tickPrecision,
		lotPrecision,
		10000)

	ob.Sync(bids, asks)
	state.instrumentData.seqNum = uint64(time.Now().UnixNano())
	state.instrumentData.orderBook = ob
	state.instrumentData.lastUpdateTime = ts

	if err := ws.Subscribe(state.security.Symbol, dydx.WSTradesV3); err != nil {
		return fmt.Errorf("error subscribing to trade stream: %v", err)
	}

	state.ws = ws

	go func(ws *dydx.Websocket, pid *actor.PID) {
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
		TickPrecision: &types.UInt64Value{Value: state.instrumentData.orderBook.TickPrecision},
		LotPrecision:  &types.UInt64Value{Value: state.instrumentData.orderBook.LotPrecision},
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
	msg := context.Message().(*dydx.WebsocketMessage)
	switch res := msg.Message.(type) {

	case error:
		return fmt.Errorf("socket error: %v", msg)

	case dydx.WSError:
		return fmt.Errorf("socket error: %v", res.Message)

	case dydx.WSOrderBookData:
		var bids, asks []gorderbook.OrderBookLevel

		for _, l := range res.Contents.Bids {
			k := uint64(math.Round(l.Price * float64(state.instrumentData.orderBook.TickPrecision)))
			if state.instrumentData.levelOffset[k] <= res.Contents.Offset {
				bids = append(bids, gorderbook.OrderBookLevel{
					Price:    l.Price,
					Quantity: l.Size,
					Bid:      true,
				})
				state.instrumentData.levelOffset[k] = res.Contents.Offset
			}
		}
		for _, l := range res.Contents.Asks {
			k := uint64(math.Round(l.Price * float64(state.instrumentData.orderBook.TickPrecision)))
			if state.instrumentData.levelOffset[k] <= res.Contents.Offset {
				asks = append(asks, gorderbook.OrderBookLevel{
					Price:    l.Price,
					Quantity: l.Size,
					Bid:      false,
				})
				state.instrumentData.levelOffset[k] = res.Contents.Offset
			}
		}

		ts := uint64(msg.Time.UnixNano()) / 1000000

		limitDelta := &models.OBL2Update{
			Levels:    []gorderbook.OrderBookLevel{},
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

	case dydx.WSTradesSubscribed:
		// Ignore
		break

	case dydx.WSTradesData:
		var aggTrade *models.AggregatedTrade
		ts := uint64(msg.Time.UnixNano()) / 1000000
		for _, trade := range res.Contents.Trades {
			aggID := (uint64(trade.CreatedAt.UnixNano()) / 1000) * 10
			// Add one to aggregatedID if it's a sell so that
			// buy and sell happening at the same time won't have the same ID
			if trade.Side == "Sell" {
				aggID += 1
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
			trade := models.Trade{
				Price:    trade.Price,
				Quantity: trade.Size,
				ID:       aggID,
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

	default:
		return fmt.Errorf("received unknown message: %s", reflect.TypeOf(msg.Message).String())
	}

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {

	if time.Now().Sub(state.lastPingTime) > 10*time.Second {
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
	if time.Now().Sub(state.instrumentData.lastHBTime) > 2*time.Second {
		// Send an empty refresh
		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			SeqNum: state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		state.instrumentData.lastHBTime = time.Now()
	}

	return nil
}