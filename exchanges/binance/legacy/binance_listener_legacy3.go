package legacy

/*
import (
	"container/list"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"github.com/quickfixgo/enum"
	fix50mdir "github.com/quickfixgo/fix50/marketdataincrementalrefresh"
	fix50mdr "github.com/quickfixgo/fix50/marketdatarequest"
	fix50mdsfr "github.com/quickfixgo/fix50/marketdatasnapshotfullrefresh"
	"github.com/shopspring/decimal"
	"gitlab.com/alphaticks/alpha-connect/messages/executor"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/gorderbook"
	"gitlab.com/alphaticks/xchanger/exchanges/binance"
	"reflect"
	"time"
)

// OBType: OBL2
// OBL2 Timestamps: ordered & consistent with sequence ID
// Trades: Impossible to infer from deltas
// Status: ready
type readSocket struct{}
type postAggTrade struct{}

type InstrumentData struct {
	ID               string
	tickPrecision    uint64
	lotPrecision     uint64
	symbol           string
	symbolCCY        utils.CCYSymbol
	orderBook        *gorderbook.OrderBookL2
	lastUpdateID     uint64
	lastUpdateTime   uint64
	lastHBTime       time.Time
	aggTrade         *fix50mdir.NoMDEntriesRepeatingGroup
	aggTradeID       uint64
	lastAggTradeTime time.Time
}

type Listener struct {
	obWs            *binance.Websocket
	tradeWs         *binance.Websocket
	wsChan          chan *binance.WebsocketMessage
	instrumentData  *InstrumentData
	executorManager *actor.PID
	logger          *log.Logger
	stashedTrades   *list.List
}

func NewListenerProducer(ID string, symbol string, tickPrecision, lotPrecision uint64) actor.Producer {
	return func() actor.Actor {
		return NewListener(ID, symbol, tickPrecision, lotPrecision)
	}
}

func NewListener(ID string, symbol string, tickPrecision, lotPrecision uint64) actor.Actor {
	instrumentData := &InstrumentData{
		ID:               ID,
		symbol:           symbol,
		tickPrecision:    tickPrecision,
		lotPrecision:     lotPrecision,
		orderBook:        nil,
		lastUpdateID:     0,
		lastUpdateTime:   0,
		lastHBTime:       time.Now(),
		aggTrade:         nil,
		aggTradeID:       0,
		lastAggTradeTime: nil,
	}
	return &Listener{
		obWs:            nil,
		tradeWs:         nil,
		wsChan:          nil,
		instrumentData:  instrumentData,
		executorManager: nil,
		logger:          nil,
		stashedTrades:   nil,
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

	case *fix50mdr.MarketDataRequest:
		if err := state.OnFIX50MarketDataRequest(context); err != nil {
			state.logger.Error("error processing FIX50MarketDataRequest", log.Error(err))
			panic(err)
		}

	case *readSocket:
		if err := state.readSocket(context); err != nil {
			state.logger.Error("error processing readSocket", log.Error(err))
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
		log.String("instrumentID", state.instrumentData.ID),
		log.String("instrumentSymbol", state.instrumentData.symbolCCY.DefaultFormat()))

	state.executorManager = actor.NewLocalPID("exchange_executor_manager")
	state.wsChan = make(chan *binance.WebsocketMessage, 10000)
	state.stashedTrades = list.New()
	symbolCCY, err := utils.NewCCYSymbol(state.instrumentData.symbol)
	if err != nil {
		return err
	}
	state.instrumentData.symbolCCY = symbolCCY

	context.Send(context.Self(), &readSocket{})

	if err := state.subscribeOrderBook(context); err != nil {
		return fmt.Errorf("error subscribing to order book: %v", err)
	}
	if err := state.subscribeTrades(context); err != nil {
		return fmt.Errorf("error subscribing to trades: %v", err)
	}

	return nil
}

func (state *Listener) Clean(context actor.Context) error {
	if state.tradeWs != nil {
		if err := state.tradeWs.Disconnect(); err != nil {
			state.logger.Info("error disconnecting socket", log.Error(err))
		}
	}
	if state.obWs != nil {
		if err := state.obWs.Disconnect(); err != nil {
			state.logger.Info("error disconnecting socket", log.Error(err))
		}
	}

	return nil
}

func (state *Listener) subscribeOrderBook(context actor.Context) error {
	if state.obWs != nil {
		_ = state.obWs.Disconnect()
	}

	obWs := binance.NewWebsocket()
	err := obWs.Connect(
		state.instrumentData.symbolCCY.Format(binance.WSSymbolFormat),
		[]string{binance.WSDepthStream100ms})
	if err != nil {
		return err
	}

	state.obWs = obWs

	time.Sleep(5 * time.Second)
	fut := context.RequestFuture(
		state.executorManager,
		&executor.GetOrderBookL2Request{Instrument: nil, RequestID: 0},
		20*time.Second)

	res, err := fut.Result()
	if err != nil {
		return fmt.Errorf("error getting OBL2")
	}
	msg := res.(*executor.GetOrderBookL2Response)
	if msg.Error != nil {
		return fmt.Errorf("error while receiving OBL2 for %v", msg.Error)
	}

	bestAsk := float64(msg.Snapshot.Asks[0].Price) / float64(msg.Snapshot.Instrument.TickPrecision)
	depth := int(((bestAsk * 1.1) - bestAsk) * float64(msg.Snapshot.Instrument.TickPrecision))

	if depth > 10000 {
		depth = 10000
	}

	ob := gorderbook.NewOrderBookL2(
		state.instrumentData.tickPrecision,
		state.instrumentData.lotPrecision,
		depth,
	)

	ob.RawSync(msg.Snapshot.Bids, msg.Snapshot.Asks)
	state.instrumentData.orderBook = ob
	state.instrumentData.lastUpdateID = msg.Snapshot.ID
	state.instrumentData.lastUpdateTime = utils.TimestampToMilli(msg.Snapshot.Timestamp)

	synced := false
	for !synced {
		if !obWs.ReadMessage() {
			return fmt.Errorf("error reading message: %v", obWs.Err)
		}
		depthData, ok := obWs.Msg.Message.(binance.WSDepthData)
		if !ok {
			return fmt.Errorf("was expecting depth data, got %s", reflect.TypeOf(obWs.Msg.Message).String())
		}

		if depthData.FinalUpdateID <= state.instrumentData.lastUpdateID {
			continue
		}

		bids, asks, err := depthData.ToBidAsk()
		if err != nil {
			return fmt.Errorf("error converting depth data: %s ", err.Error())
		}
		for _, bid := range bids {
			ob.UpdateOrderBookLevel(bid)
		}
		for _, ask := range asks {
			ob.UpdateOrderBookLevel(ask)
		}

		state.instrumentData.lastUpdateID = depthData.FinalUpdateID
		state.instrumentData.lastUpdateTime = uint64(obWs.Msg.Time.UnixNano() / 1000000)

		synced = true
	}

	go func(ws *binance.Websocket) {
		for ws.ReadMessage() {
			state.wsChan <- ws.Msg
		}
	}(state.obWs)

	return nil
}

func (state *Listener) subscribeTrades(context actor.Context) error {
	if state.tradeWs != nil {
		_ = state.tradeWs.Disconnect()
	}
	tradeWs := binance.NewWebsocket()
	err := tradeWs.Connect(
		state.instrumentData.symbolCCY.Format(binance.WSSymbolFormat),
		[]string{binance.WSTradeStream})
	if err != nil {
		return err
	}
	state.tradeWs = tradeWs

	go func(ws *binance.Websocket) {
		for ws.ReadMessage() {
			state.wsChan <- ws.Msg
		}
	}(state.tradeWs)

	return nil
}

func (state *Listener) OnFIX50MarketDataRequest(context actor.Context) error {
	msg := context.Message().(*fix50mdr.MarketDataRequest)

	response := fix50mdsfr.New()
	response.SetSymbol(state.instrumentData.symbol)
	reqID, err := msg.GetMDReqID()
	if err != nil {
		return fmt.Errorf("error getting MDReqID field: %v", err)
	}
	response.SetMDReqID(reqID)
	entries := fix50mdsfr.NewNoMDEntriesRepeatingGroup()

	entryTypes, err := msg.GetNoMDEntryTypes()
	if err != nil {
		return fmt.Errorf("error getting entry types field: %v", err)
	}
	for i := 0; i < entryTypes.Len(); i++ {
		entryType := entryTypes.Get(i)
		typ, err := entryType.GetMDEntryType()
		if err != nil {
			return fmt.Errorf("error getting type field: %v", err)
		}
		switch typ {
		case enum.MDEntryType_BID:
			depth, err := msg.GetMarketDepth()
			if err != nil {
				return fmt.Errorf("error getting market depth field: %v", err)
			}
			bids := state.instrumentData.orderBook.GetAbsoluteRawBids(depth)
			for _, b := range bids {
				entry := entries.Add()
				entry.SetMDEntryType(enum.MDEntryType_BID)
				entry.SetMDEntryPx(
					decimal.New(int64(b.Price), int32(state.instrumentData.tickPrecision)),
					int32(state.instrumentData.tickPrecision))
				entry.SetMDEntrySize(
					decimal.New(int64(b.Quantity), int32(state.instrumentData.lotPrecision)),
					int32(state.instrumentData.lotPrecision))
			}

		case enum.MDEntryType_OFFER:
			depth, err := msg.GetMarketDepth()
			if err != nil {
				return fmt.Errorf("error getting market depth field: %v", err)
			}
			asks := state.instrumentData.orderBook.GetAbsoluteRawBids(depth)
			for _, a := range asks {
				entry := entries.Add()
				entry.SetMDEntryType(enum.MDEntryType_OFFER)
				entry.SetMDEntryPx(
					decimal.New(int64(a.Price), int32(state.instrumentData.tickPrecision)),
					int32(state.instrumentData.tickPrecision))
				entry.SetMDEntrySize(
					decimal.New(int64(a.Quantity), int32(state.instrumentData.lotPrecision)),
					int32(state.instrumentData.lotPrecision))
			}
		}
	}

	context.Respond(&response)
	return nil
}

func (state *Listener) readSocket(context actor.Context) error {
	select {
	case msg := <-state.wsChan:
		switch msg.Message.(type) {

		case error:
			return fmt.Errorf("socket error: %v", msg)

		case binance.WSDepthData:
			depthData := msg.Message.(binance.WSDepthData)

			// change event time
			depthData.EventTime = uint64(msg.Time.UnixNano()) / 1000000
			err := state.onDepthData(context, depthData)
			if err != nil {
				state.logger.Info("error processing depth data for "+depthData.Symbol,
					log.Error(err))
				// Stop the socket, we will restart instrument at the end
				if err := state.obWs.Disconnect(); err != nil {
					state.logger.Info("error disconnecting from socket", log.Error(err))
				}
			}

		case binance.WSTradeData:
			tradeData := msg.Message.(binance.WSTradeData)
			var aggregateID uint64
			if tradeData.MarketSell {
				aggregateID = uint64(tradeData.SellerOrderID)
			} else {
				aggregateID = uint64(tradeData.BuyerOrderID)
			}

			var entry fix50mdir.NoMDEntries
			var tradeTime time.Time

			if state.instrumentData.aggTradeID != aggregateID {
				if state.instrumentData.lastAggTradeTime.Equal(msg.Time) {
					tradeTime = msg.Time.Add(time.Millisecond)
				} else {
					tradeTime = msg.Time
				}

				// Create a new one
				aggTrade := fix50mdir.NewNoMDEntriesRepeatingGroup()
				entry = aggTrade.Add()
				state.instrumentData.aggTradeID = aggregateID
				state.instrumentData.aggTrade = &aggTrade
				state.stashedTrades.PushBack(&aggTrade)
				go func(pid *actor.PID) {
					time.Sleep(21 * time.Millisecond)
					context.Send(pid, &postAggTrade{})
				}(context.Self())
			} else {
				entry = state.instrumentData.aggTrade.Add()
				// Force same trade time for aggregate trade
				tradeTime = state.instrumentData.lastAggTradeTime
			}

			entry.SetMDUpdateAction(enum.MDUpdateAction_NEW)
			entry.SetMDEntryType(enum.MDEntryType_TRADE)
			entry.SetMDEntryID(fmt.Sprintf("%d", tradeData.TradeID))
			entry.SetSymbol(state.instrumentData.symbol)
			rawPrice := int64(tradeData.Price * float64(state.instrumentData.tickPrecision))
			entry.SetMDEntryPx(
				decimal.New(rawPrice, int32(state.instrumentData.tickPrecision)),
				int32(state.instrumentData.tickPrecision))
			rawSize := int64(tradeData.Quantity * float64(state.instrumentData.lotPrecision))
			entry.SetMDEntrySize(
				decimal.New(rawSize, int32(state.instrumentData.lotPrecision)),
				int32(state.instrumentData.lotPrecision))
			entry.SetMDEntryDate(tradeTime.Format(utils.FIX_UTCDateOnly_LAYOUT))
			entry.SetMDEntryTime(tradeTime.Format(utils.FIX_UTCTimeOnly_LAYOUT))
		}

		if err := state.checkSockets(context); err != nil {
			return fmt.Errorf("error checking sockets: %v", err)
		}
		state.postHeartBeat(context)
		context.Send(context.Self(), &readSocket{})
		return nil

	case <-time.After(1 * time.Second):
		if err := state.checkSockets(context); err != nil {
			return fmt.Errorf("error checking sockets: %v", err)
		}
		state.postHeartBeat(context)
		context.Send(context.Self(), &readSocket{})
		return nil
	}
}

func (state *Listener) onDepthData(context actor.Context, depthData binance.WSDepthData) error {

	symbol := depthData.Symbol

	// Skip depth that are younger than OB
	if depthData.FinalUpdateID <= state.instrumentData.lastUpdateID {
		return nil
	}

	// Check depth continuity
	if state.instrumentData.lastUpdateID+1 != depthData.FirstUpdateID {
		return fmt.Errorf("got wrong sequence ID for %s: %d, %d",
			symbol, state.instrumentData.lastUpdateID, depthData.FirstUpdateID)
	}

	bids, asks, err := depthData.ToRawBidAsk(state.instrumentData.tickPrecision, state.instrumentData.lotPrecision)
	if err != nil {
		return fmt.Errorf("error converting depth data: %s ", err.Error())
	}

	rg := fix50mdir.NewNoMDEntriesRepeatingGroup()
	for _, bid := range bids {
		entry := rg.Add()
		entry.SetSymbol(state.instrumentData.symbol)
		entry.SetMDEntryType(enum.MDEntryType_BID)
		entry.SetMDEntryPx(decimal.New(int64(bid.Price), int32(state.instrumentData.tickPrecision)), int32(state.instrumentData.tickPrecision))
		entry.SetMDEntrySize(decimal.New(int64(bid.Quantity), int32(state.instrumentData.lotPrecision)), int32(state.instrumentData.lotPrecision))
		state.instrumentData.orderBook.UpdateRawOrderBookLevel(bid)
	}

	for _, ask := range asks {
		entry := rg.Add()
		entry.SetSymbol(state.instrumentData.symbol)
		entry.SetMDEntryType(enum.MDEntryType_OFFER)
		entry.SetMDEntryPx(decimal.New(int64(ask.Price), int32(state.instrumentData.tickPrecision)), int32(state.instrumentData.tickPrecision))
		entry.SetMDEntrySize(decimal.New(int64(ask.Quantity), int32(state.instrumentData.lotPrecision)), int32(state.instrumentData.lotPrecision))
		state.instrumentData.orderBook.UpdateRawOrderBookLevel(ask)
	}

	if state.instrumentData.orderBook.Crossed() {
		return fmt.Errorf("crossed order book")
	}

	state.instrumentData.lastUpdateID = depthData.FinalUpdateID
	state.instrumentData.lastUpdateTime = depthData.EventTime

	refresh := fix50mdir.New()
	refresh.SetMDBookType(enum.MDBookType_PRICE_DEPTH)
	refresh.SetNoMDEntries(rg)

	context.Send(context.Parent(), &refresh)

	state.instrumentData.lastHBTime = time.Now()

	return nil
}

func (state *Listener) checkSockets(context actor.Context) error {
	// TODO ping or HB ?
	if state.obWs.Err != nil || !state.obWs.Connected {
		if state.obWs.Err != nil {
			state.logger.Info("error on socket", log.Error(state.obWs.Err))
		}
		if err := state.subscribeOrderBook(context); err != nil {
			return fmt.Errorf("error subscribing to instrument: %v", err)
		}
	}

	if state.tradeWs.Err != nil || !state.tradeWs.Connected {
		if state.tradeWs.Err != nil {
			state.logger.Info("error on socket", log.Error(state.tradeWs.Err))
		}
		if err := state.subscribeTrades(context); err != nil {
			return fmt.Errorf("error subscribing to instrument: %v", err)
		}
	}

	return nil
}

func (state *Listener) postHeartBeat(context actor.Context) {
	// If haven't sent anything for 2 seconds, send heartbeat
	if time.Now().Sub(state.instrumentData.lastHBTime) > 2*time.Second {
		//topic := fmt.Sprintf("%s/HEARTBEAT", state.instrument.DefaultFormat())
		// TODO HB ?
		context.Send(context.Parent(), nil)
		state.instrumentData.lastHBTime = time.Now()
	}
}

func (state *Listener) postAggTrade(context actor.Context) {
	for el := state.stashedTrades.Front(); el != nil; el = state.stashedTrades.Front() {
		trd := el.Value.(*fix50mdir.NoMDEntriesRepeatingGroup)
		entry := trd.Get(0)
		entryDate, _ := entry.GetMDEntryDate()
		entryTime, _ := entry.GetMDEntryTime()
		ts, _ := time.Parse(utils.FIX_UTCDateTime_LAYOUT, entryDate+"-"+entryTime)
		if time.Now().Sub(ts) > 20*time.Millisecond {
			refresh := fix50mdir.New()
			refresh.SetNoMDEntries(*trd)
			context.Send(context.Parent(), &refresh)
			// At this point, the state.instrumentData.aggTrade can be our trade, or it can be a new one
			if state.instrumentData.aggTrade == trd {
				state.instrumentData.aggTrade = nil
				state.instrumentData.aggTradeID = 0
			}
			state.stashedTrades.Remove(el)
		} else {
			break
		}
	}
}
*/
