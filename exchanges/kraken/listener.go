package kraken

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
	"gitlab.com/alphaticks/xchanger/exchanges/kraken"
	xchangerUtils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math"
	"reflect"
	"sort"
	"time"
)

type checkSockets struct{}

type InstrumentData struct {
	orderBook      *gorderbook.OrderBookL2
	seqNum         uint64
	lastUpdateTime uint64
	lastHBTime     time.Time
	lastAggTradeTs uint64
}

// OBType: OBL2
// OBL2 Timestamps: Per server, no ID so timestamps are the ID no risk of disorder
// Trades: Impossible to infer from delta
// Status: Not ready, different ID occurs for different listeners due to different obdelta received

// Note: on the kraken website, it can display wrong colors for the trades, but the
// websocket trades are correct

type Listener struct {
	ws             *kraken.Websocket
	securityID     uint64
	security       *models.Security
	executor       *actor.PID
	dialerPool     *xchangerUtils.DialerPool
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
	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.KRAKEN.Name+"_executor")

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
		orderBook:      nil,
		seqNum:         uint64(time.Now().UnixNano()),
		lastUpdateTime: 0,
		lastHBTime:     time.Now(),
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
		state.ws = nil
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

	ws := kraken.NewWebsocket()
	if err := ws.Connect(state.dialerPool.GetDialer()); err != nil {
		return fmt.Errorf("error connecting to kraken websocket: %v", err)
	}

	if err := ws.SubscribeDepth([]string{state.security.Symbol}, kraken.WSDepth1000); err != nil {
		return fmt.Errorf("error subscribing to depth stream")
	}

	for {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}
		if _, ok := ws.Msg.Message.(kraken.WSHeartBeat); ok {
			continue
		}
		s, ok := ws.Msg.Message.(kraken.WSSystemStatus)
		if !ok {
			return fmt.Errorf("was expecting WSSystemStatus, got %s", reflect.TypeOf(ws.Msg.Message).String())
		}
		if s.Status != "online" {
			return fmt.Errorf("was expecting online status, got %s", s.Status)
		}
		break
	}

	for {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}
		if _, ok := ws.Msg.Message.(kraken.WSHeartBeat); ok {
			continue
		}
		su, ok := ws.Msg.Message.(kraken.WSSubscriptionStatus)
		if !ok {
			return fmt.Errorf("was expecting WSSubscriptionStatus, got %s", reflect.TypeOf(ws.Msg.Message).String())
		}
		if su.Status != "subscribed" {
			return fmt.Errorf("was expecting subscribed status, got %s", su.Status)
		}
		break
	}

	var obData kraken.WSOrderBookL2
	for {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading message: %v", ws.Err)
		}
		if _, ok := ws.Msg.Message.(kraken.WSHeartBeat); ok {
			continue
		}
		var ok bool
		obData, ok = ws.Msg.Message.(kraken.WSOrderBookL2)
		if !ok {
			return fmt.Errorf("was expecting WSOrderBookL2, got %s", reflect.TypeOf(ws.Msg.Message).String())
		}
		break
	}

	bids, asks := obData.ToBidAsk()

	//bestAsk := float64(asks[0].Price) / float64(state.instruments[obData.Symbol].instrument.TickPrecision)
	// Allow a 10% price variation
	//depth := int(((bestAsk * 1.1) - bestAsk) * float64(state.instruments[obData.Symbol].instrument.TickPrecision))

	var maxID uint64 = 0
	for _, ask := range obData.Asks {
		if ask.Time > maxID {
			maxID = ask.Time
		}
	}
	for _, bid := range obData.Bids {
		if bid.Time > maxID {
			maxID = bid.Time
		}
	}
	tickPrecision := uint64(math.Ceil(1. / state.security.MinPriceIncrement.Value))
	lotPrecision := uint64(math.Ceil(1. / state.security.RoundLot.Value))
	ob := gorderbook.NewOrderBookL2(tickPrecision, lotPrecision, 10000)

	ob.Sync(bids, asks)
	if ob.Crossed() {
		return fmt.Errorf("cossed order book")
	}
	ts := uint64(ws.Msg.ClientTime.UnixNano() / 1000000)
	state.instrumentData.orderBook = ob
	state.instrumentData.seqNum = uint64(time.Now().UnixNano())
	state.instrumentData.lastUpdateTime = ts

	if err := ws.SubscribeTrade([]string{state.security.Symbol}); err != nil {
		return fmt.Errorf("error subscribing to trade stream")
	}

	state.ws = ws

	go func(ws *kraken.Websocket, pid *actor.PID) {
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

	case kraken.WSOrderBookL2Update:
		obData := msg.Message.(kraken.WSOrderBookL2Update)

		nLevels := len(obData.Bids) + len(obData.Asks)

		if nLevels == 0 {
			break
		}
		instr := state.instrumentData

		ts := uint64(msg.ClientTime.UnixNano() / 1000000)

		obDelta := &models.OBL2Update{
			Levels:    []*gmodels.OrderBookLevel{},
			Timestamp: utils.MilliToTimestamp(ts),
			Trade:     false,
		}

		lvlIdx := 0
		for _, bid := range obData.Bids {
			level := &gmodels.OrderBookLevel{
				Price:    bid.Price,
				Quantity: bid.Amount,
				Bid:      true,
			}
			lvlIdx += 1
			instr.orderBook.UpdateOrderBookLevel(level)
			obDelta.Levels = append(obDelta.Levels, level)
		}

		for _, ask := range obData.Asks {
			level := &gmodels.OrderBookLevel{
				Price:    ask.Price,
				Quantity: ask.Amount,
				Bid:      false,
			}
			lvlIdx += 1
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
		state.instrumentData.lastUpdateTime = utils.TimestampToMilli(obDelta.Timestamp)

	case kraken.WSTradeUpdate:
		tradeUpdate := msg.Message.(kraken.WSTradeUpdate)
		// order trade by timestamps
		// if trades are on same side, aggregate. We aggregate because otherwise
		// we will have two trades on the same side with the same timestamp as
		// we use same current timestamp for timestamp of all the trades in the array
		// the timestamp of the first trade is going to be the aggregateID and each
		// trade in the aggregate will have as ID aggregateID + index of trade in aggregate

		if len(tradeUpdate.Trades) == 0 {
			break
		}

		ts := uint64(msg.ClientTime.UnixNano() / 1000000)

		sort.Slice(tradeUpdate.Trades, func(i, j int) bool {
			return tradeUpdate.Trades[i].Time < tradeUpdate.Trades[j].Time
		})
		tradeID := tradeUpdate.Trades[0].Time

		var buyTrades []kraken.WSTrade
		var sellTrades []kraken.WSTrade

		for _, t := range tradeUpdate.Trades {
			if t.Side == "s" {
				sellTrades = append(sellTrades, t)
			} else {
				buyTrades = append(buyTrades, t)
			}
		}

		if len(buyTrades) > 0 {
			if ts <= state.instrumentData.lastAggTradeTs {
				ts = state.instrumentData.lastAggTradeTs + 1
			}
			aggBuyTrade := &models.AggregatedTrade{
				Bid:         false,
				Timestamp:   utils.MilliToTimestamp(ts),
				AggregateID: tradeID,
				Trades:      nil,
			}
			for _, trade := range buyTrades {
				trd := &models.Trade{
					Price:    trade.Price,
					Quantity: trade.Volume,
					ID:       tradeID,
				}
				tradeID += 1
				aggBuyTrade.Trades = append(aggBuyTrade.Trades, trd)
			}
			context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
				Trades: []*models.AggregatedTrade{aggBuyTrade},
				SeqNum: state.instrumentData.seqNum + 1,
			})
			state.instrumentData.seqNum += 1
			state.instrumentData.lastAggTradeTs = ts
		}

		if len(sellTrades) > 0 {
			if ts <= state.instrumentData.lastAggTradeTs {
				ts = state.instrumentData.lastAggTradeTs + 1
			}
			aggSellTrade := &models.AggregatedTrade{
				Bid:         true,
				Timestamp:   utils.MilliToTimestamp(ts),
				AggregateID: tradeID,
				Trades:      nil,
			}
			for _, trade := range sellTrades {
				trd := &models.Trade{
					Price:    trade.Price,
					Quantity: trade.Volume,
					ID:       tradeID,
				}
				tradeID += 1
				aggSellTrade.Trades = append(aggSellTrade.Trades, trd)
			}
			context.Send(context.Parent(), &messages.MarketDataIncrementalRefresh{
				Trades: []*models.AggregatedTrade{aggSellTrade},
				SeqNum: state.instrumentData.seqNum + 1,
			})
			state.instrumentData.seqNum += 1
			state.instrumentData.lastAggTradeTs = ts
		}
	}

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {
	if time.Since(state.lastPingTime) > 5*time.Second {
		// "Ping" by resubscribing to the topic
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
