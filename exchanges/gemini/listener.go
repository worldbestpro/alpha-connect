package gemini

import (
	"container/list"
	"errors"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/gorderbook"
	"gitlab.com/alphaticks/xchanger"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges/gemini"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math"
	"reflect"
	"time"
)

type checkSockets struct{}
type postAggTrade struct{}

type InstrumentData struct {
	orderBook      *gorderbook.OrderBookL2
	seqNum         uint64
	lastUpdateTime uint64
	lastHBTime     time.Time
	aggTrade       *models.AggregatedTrade
	lastAggTradeTs uint64
}

type Listener struct {
	ws             *gemini.Websocket
	securityID     uint64
	security       *models.Security
	executor       *actor.PID
	dialerPool     *xchangerUtils.DialerPool
	instrumentData *InstrumentData
	logger         *log.Logger
	stashedTrades  *list.List
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

	case *postAggTrade:
		state.postAggTrade(context)
	}
}

func (state *Listener) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()),
		log.String("security-id", fmt.Sprintf("%d", state.securityID)))
	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.GEMINI.Name+"_executor")

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
	state.stashedTrades = list.New()

	state.instrumentData = &InstrumentData{
		orderBook:      nil,
		seqNum:         uint64(time.Now().UnixNano()),
		lastUpdateTime: 0,
		lastHBTime:     time.Now(),
		aggTrade:       nil,
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

	ws := gemini.NewWebsocket()
	if err := ws.Connect(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to gemini websocket: %v", err)
	}

	if err := ws.Subscribe(state.security.Symbol, gemini.WSL2Feed); err != nil {
		return fmt.Errorf("error subscribing to l2 feed")
	}

	if !ws.ReadMessage() {
		return fmt.Errorf("error reading message: %v", ws.Err)
	}
	update, ok := ws.Msg.Message.(gemini.WSL2Updates)
	if !ok {
		return fmt.Errorf("was expecting WSL2Updates, got %s", reflect.TypeOf(ws.Msg.Message).String())
	}

	bids, asks := update.ToBidAsk()

	// TODO
	/*
		bestAsk := float64(asks[0]) / float64(msg.Snapshot.Instrument.TickPrecision)
		depth := int(((bestAsk * 1.1) - bestAsk) * float64(msg.Snapshot.Instrument.TickPrecision))

		if depth > 10000 {
			depth = 10000
		}
	*/
	tickPrecision := uint64(math.Ceil(1. / state.security.MinPriceIncrement.Value))
	lotPrecision := uint64(math.Ceil(1. / state.security.RoundLot.Value))
	ob := gorderbook.NewOrderBookL2(
		tickPrecision,
		lotPrecision,
		10000)
	ob.Sync(bids, asks)
	if ob.Crossed() {
		return fmt.Errorf("crossed order book")
	}
	ts := uint64(ws.Msg.ClientTime.UnixNano() / 1000000)
	state.instrumentData.orderBook = ob
	state.instrumentData.lastUpdateTime = ts
	state.instrumentData.seqNum = uint64(time.Now().UnixNano())

	state.ws = ws

	go func(ws *gemini.Websocket, pid *actor.PID) {
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
		return fmt.Errorf("socket error: %v", msg)

	case gemini.WSL2Updates:
		updates := msg.Message.(gemini.WSL2Updates)

		instr := state.instrumentData

		bids, asks := updates.ToBidAsk()
		ts := uint64(msg.ClientTime.UnixNano() / 1000000)

		obDelta := &models.OBL2Update{
			Levels:    nil,
			Timestamp: utils.MilliToTimestamp(ts),
			Trade:     false,
		}

		for _, bid := range bids {
			instr.orderBook.UpdateOrderBookLevel(bid)
			obDelta.Levels = append(obDelta.Levels, bid)
		}
		for _, ask := range asks {
			instr.orderBook.UpdateOrderBookLevel(ask)
			obDelta.Levels = append(obDelta.Levels, ask)
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
		instr.lastUpdateTime = uint64(msg.ClientTime.UnixNano() / 1000000)

	case gemini.WSTrade:
		trade := msg.Message.(gemini.WSTrade)
		ts := uint64(msg.ClientTime.UnixNano()) / 1000000

		aggID := trade.Timestamp * 10
		if trade.Side == "sell" {
			aggID += 1
		}
		if state.instrumentData.aggTrade == nil || state.instrumentData.aggTrade.AggregateID != aggID {
			// ensure increasing timestamp
			if ts <= state.instrumentData.lastAggTradeTs {
				ts = state.instrumentData.lastAggTradeTs + 1
			}
			aggTrade := &models.AggregatedTrade{
				Bid:         trade.Side == "sell",
				Timestamp:   utils.MilliToTimestamp(ts),
				AggregateID: aggID,
				Trades:      nil,
			}
			state.instrumentData.aggTrade = aggTrade
			state.instrumentData.lastAggTradeTs = ts

			// Stash the aggTrade
			state.stashedTrades.PushBack(aggTrade)
			// start the timer on trade creation, it will publish the trade in 20 ms
			go func(pid *actor.PID) {
				time.Sleep(21 * time.Millisecond)
				context.Send(pid, &postAggTrade{})
			}(context.Self())
		}
		state.instrumentData.aggTrade.Trades = append(
			state.instrumentData.aggTrade.Trades,
			&models.Trade{
				Price:    trade.Price,
				Quantity: trade.Quantity,
				ID:       trade.EventID,
			})
	}

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {
	// Don't re-subscribe as ping, we have heartbeats
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

func (state *Listener) postAggTrade(context actor.Context) {
	nowMilli := uint64(time.Now().UnixNano() / 1000000)

	for el := state.stashedTrades.Front(); el != nil; el = state.stashedTrades.Front() {
		trd := el.Value.(*models.AggregatedTrade)
		if trd != nil && nowMilli-utils.TimestampToMilli(trd.Timestamp) > 20 {
			context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
				Trades: []*models.AggregatedTrade{trd},
				SeqNum: state.instrumentData.seqNum + 1,
			})
			state.instrumentData.seqNum += 1
			// At this point, the state.instrumentData.aggTrade can be our trade, or it can be a new one
			if state.instrumentData.aggTrade == trd {
				state.instrumentData.aggTrade = nil
			}
			state.stashedTrades.Remove(el)
		} else {
			break
		}
	}
}
