package bybits

import (
	"errors"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/gorderbook"
	gmodels "gitlab.com/alphaticks/gorderbook/gorderbook.models"
	"gitlab.com/alphaticks/xchanger"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges/bybits"
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
	mergedBook          *gorderbook.OrderBookL2
	seqNum              uint64
	lastUpdateTime      uint64
	lastHBTime          time.Time
	lastAggTradeTs      uint64
	lastLiquidationTime uint64
}

type Listener struct {
	ws             *bybits.Websocket
	security       *models.Security
	securityID     uint64
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
	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.BYBITS.Name+"_executor")

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

	ws := bybits.NewWebsocket()
	if err := ws.Connect(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to bybit websocket: %v", err)
	}

	dumpScale := -int(math.Round(math.Log10(state.security.MinPriceIncrement.Value))) - 5
	for dumpScale < 0 {
		if err := ws.SubscribeMergedDepth(state.security.Symbol, dumpScale); err != nil {
			return fmt.Errorf("error subscribing to merged orderbook for %s", state.security.Symbol)
		}
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}
		res, ok := ws.Msg.Message.(bybits.WSResponse)
		// !ok means we are good
		if !ok || res.Code == "0" {
			break
		} else {
			dumpScale += 1
		}
	}

	if err := ws.Subscribe(state.security.Symbol, bybits.WSDiffDepthTopic); err != nil {
		return fmt.Errorf("error subscribing to orderbook for %s", state.security.Symbol)
	}

	var ob *gorderbook.OrderBookL2
	nTries := 0
	for nTries < 100 {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}

		switch msg := ws.Msg.Message.(type) {
		case bybits.WSDiffDepths:
			bids, asks := msg[0].ToBidAsk()
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
			state.instrumentData.mergedBook = gorderbook.NewOrderBookL2(
				tickPrecision,
				lotPrecision,
				10000)
			state.instrumentData.mergedBook.Sync(nil, nil)
			state.instrumentData.lastUpdateTime = ts
			state.instrumentData.seqNum = uint64(time.Now().UnixNano())
			nTries = 100

		case bybits.WSResponse:
			if msg.Code != "0" {
				err := fmt.Errorf("error getting orderbook: %s", msg.Msg)
				return err
			}
		}
		nTries += 1
	}

	if ob == nil {
		return fmt.Errorf("error getting orderbook")
	}

	if err := ws.Subscribe(state.security.Symbol, bybits.WSTradeTopic); err != nil {
		return fmt.Errorf("error subscribing to trades for %s", state.security.Symbol)
	}

	state.ws = ws

	go func(ws *bybits.Websocket, pid *actor.PID) {
		for ws.ReadMessage() {
			context.Send(pid, ws.Msg)
		}
	}(ws, context.Self())

	return nil
}

func (state *Listener) OnMarketDataRequest(context actor.Context) error {
	msg := context.Message().(*messages.MarketDataRequest)
	bids := state.instrumentData.mergedBook.GetBids(0)
	asks := state.instrumentData.mergedBook.GetAsks(0)
	bids = append(bids, state.instrumentData.orderBook.GetBids(0)...)
	asks = append(asks, state.instrumentData.orderBook.GetAsks(0)...)
	snapshot := &models.OBL2Snapshot{
		Bids:          bids,
		Asks:          asks,
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
	wsmsg := context.Message().(*xchanger.WebsocketMessage)
	if state.ws == nil || wsmsg.WSID != state.ws.ID {
		return nil
	}
	switch msg := wsmsg.Message.(type) {
	case error:
		return fmt.Errorf("OB socket error: %v", msg)

	case bybits.WSDiffDepths:
		ts := uint64(wsmsg.ClientTime.UnixNano() / 1000000)

		instr := state.instrumentData

		obDelta := &models.OBL2Update{
			Levels:    nil,
			Timestamp: utils.MilliToTimestamp(ts),
			Trade:     false,
		}

		for _, dd := range msg {
			for _, a := range dd.Asks {
				level := &gmodels.OrderBookLevel{
					Price:    a.Price,
					Quantity: a.Size,
					Bid:      false,
				}
				instr.orderBook.UpdateOrderBookLevel(level)
				obDelta.Levels = append(obDelta.Levels, level)
			}
			for _, b := range dd.Bids {
				level := &gmodels.OrderBookLevel{
					Price:    b.Price,
					Quantity: b.Size,
					Bid:      true,
				}
				instr.orderBook.UpdateOrderBookLevel(level)
				obDelta.Levels = append(obDelta.Levels, level)
			}
		}

		if instr.orderBook.Crossed() {
			state.logger.Info("crossed orderbook", log.Error(errors.New("crossed")))
			return state.subscribeInstrument(context)
		}
		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			UpdateL2: obDelta,
			SeqNum:   state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		instr.lastUpdateTime = ts

	case bybits.WSMergedDepths:
		ts := uint64(wsmsg.ClientTime.UnixNano() / 1000000)

		bids := state.instrumentData.orderBook.GetBids(-1)
		worstBid := bids[len(bids)-1].Price
		asks := state.instrumentData.orderBook.GetAsks(-1)
		worstAsk := asks[len(asks)-1].Price

		bids = nil
		asks = nil

		bbids, basks := msg[0].ToBidAsk()
		for _, b := range bbids {
			if b.Price < worstBid {
				// Add to book
				bids = append(bids, b)
			}
		}
		for _, a := range basks {
			if a.Price > worstAsk {
				// Add to book
				asks = append(asks, a)
			}
		}

		mergedBook := gorderbook.NewOrderBookL2(
			state.instrumentData.mergedBook.TickPrecision,
			state.instrumentData.mergedBook.LotPrecision,
			state.instrumentData.mergedBook.Depth)

		mergedBook.Sync(bids, asks)

		levels := state.instrumentData.mergedBook.Diff(mergedBook)
		state.instrumentData.mergedBook = mergedBook

		context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
			UpdateL2: &models.OBL2Update{
				Levels:    levels,
				Timestamp: utils.MilliToTimestamp(ts),
				Trade:     false,
			},
			SeqNum: state.instrumentData.seqNum + 1,
		})
		state.instrumentData.seqNum += 1
		state.instrumentData.lastUpdateTime = ts

	case bybits.WSTrades:
		ts := uint64(wsmsg.ClientTime.UnixNano() / 1000000)

		sort.Slice(msg, func(i, j int) bool {
			return msg[i].Timestamp < msg[j].Timestamp
		})

		var aggTrade *models.AggregatedTrade
		var aggIDLast uint64 = 0
		for _, trade := range msg {
			aggID := trade.Timestamp * 10
			// do that so new agg trade if side changes
			if trade.Quantity < 0 {
				aggID += 1
			}
			if aggTrade == nil || aggIDLast != aggID {
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
					Bid:         !trade.Buy,
					Timestamp:   utils.MilliToTimestamp(ts),
					AggregateID: trade.TradeID,
					Trades:      nil,
				}
				aggIDLast = aggID
			}

			trd := &models.Trade{
				Price:    trade.Price,
				Quantity: trade.Quantity,
				ID:       trade.TradeID,
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

	case bybits.WSResponse:
		if msg.Code != "0" {
			return fmt.Errorf("received a non-sucess response: %s %s", msg.Msg, msg.Desc)
		}

	case bybits.WSPong:

	default:
		state.logger.Info("received unknown message",
			log.String("message_type",
				reflect.TypeOf(wsmsg.Message).String()))
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
