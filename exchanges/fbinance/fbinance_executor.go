package fbinance

import (
	"encoding/json"
	"fmt"
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/log"
	"github.com/gogo/protobuf/types"
	"gitlab.com/alphaticks/alphac/enum"
	"gitlab.com/alphaticks/alphac/exchanges/interface"
	"gitlab.com/alphaticks/alphac/jobs"
	"gitlab.com/alphaticks/alphac/models"
	"gitlab.com/alphaticks/alphac/models/messages"
	"gitlab.com/alphaticks/alphac/utils"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/bitmex"
	"gitlab.com/alphaticks/xchanger/exchanges/fbinance"
	"io/ioutil"
	"math"
	"net/http"
	"reflect"
	"time"
)

// Execute api calls
// Contains rate limit
// Spawn a query actor for each request
// and pipe its result back

// 429 rate limit
// 418 IP ban

// The role of a Binance Executor is to
// process api request

// The global rate limit is per IP and the orderRateLimit is per
// account.

type Executor struct {
	client               *http.Client
	securities           map[uint64]*models.Security
	symbolToSec          map[string]*models.Security
	minuteOrderRateLimit *exchanges.RateLimit
	globalRateLimit      *exchanges.RateLimit
	queryRunner          *actor.PID
	logger               *log.Logger
}

func NewExecutor() actor.Actor {
	return &Executor{
		client:               nil,
		minuteOrderRateLimit: nil,
		globalRateLimit:      nil,
		queryRunner:          nil,
		logger:               nil,
	}
}

func (state *Executor) Receive(context actor.Context) {
	_interface.ExchangeExecutorReceive(state, context)
}

func (state *Executor) GetLogger() *log.Logger {
	return state.logger
}

func (state *Executor) Initialize(context actor.Context) error {
	state.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))

	props := actor.PropsFromProducer(func() actor.Actor {
		return jobs.NewAPIQuery(state.client)
	})
	state.queryRunner = context.Spawn(props)

	request, weight, err := fbinance.GetExchangeInfo()

	future := context.RequestFuture(state.queryRunner, &jobs.PerformQueryRequest{Request: request}, 10*time.Second)
	res, err := future.Result()
	if err != nil {
		return err
	}
	queryResponse := res.(*jobs.PerformQueryResponse)
	if queryResponse.StatusCode != 200 {
		return fmt.Errorf("error getting exchange info: status code %d", queryResponse.StatusCode)
	}

	var exchangeInfo fbinance.ExchangeInfo
	err = json.Unmarshal(queryResponse.Response, &exchangeInfo)
	if err != nil {
		return fmt.Errorf("error decoding query response: %v", err)
	}
	if exchangeInfo.Code != 0 {
		return fmt.Errorf("error getting exchange info: %s", exchangeInfo.Msg)
	}

	// Initialize rate limit
	for _, rateLimit := range exchangeInfo.RateLimits {
		if rateLimit.RateLimitType == "ORDERS" {
			if rateLimit.Interval == "MINUTE" {
				state.minuteOrderRateLimit = exchanges.NewRateLimit(rateLimit.Limit, time.Minute)
			}
		} else if rateLimit.RateLimitType == "REQUEST_WEIGHT" {
			state.globalRateLimit = exchanges.NewRateLimit(rateLimit.Limit, time.Minute)
		}
	}
	if state.minuteOrderRateLimit == nil || state.globalRateLimit == nil {
		return fmt.Errorf("unable to set minute or global rate limit")
	}

	// Update rate limit with weight from the current exchange info fetch
	state.globalRateLimit.Request(weight)
	return state.UpdateSecurityList(context)
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, weight, err := fbinance.GetExchangeInfo()
	if err != nil {
		return err
	}

	if state.globalRateLimit.IsRateLimited() {
		return fmt.Errorf("rate limited")
	}

	state.globalRateLimit.Request(weight)
	resp, err := state.client.Do(request)
	if err != nil {
		return err
	}
	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			err := fmt.Errorf(
				"http client error: %d %s",
				resp.StatusCode,
				string(response))
			return err
		} else if resp.StatusCode >= 500 {
			err := fmt.Errorf(
				"http server error: %d %s",
				resp.StatusCode,
				string(response))
			return err
		} else {
			err := fmt.Errorf("%d %s",
				resp.StatusCode,
				string(response))
			return err
		}
	}
	var exchangeInfo fbinance.ExchangeInfo
	err = json.Unmarshal(response, &exchangeInfo)
	if err != nil {
		err = fmt.Errorf(
			"error unmarshaling response: %v",
			err)
		return err
	}
	if exchangeInfo.Code != 0 {
		err = fmt.Errorf(
			"fbinance api error: %d %s",
			exchangeInfo.Code,
			exchangeInfo.Msg)
		return err
	}

	var securities []*models.Security
	for _, symbol := range exchangeInfo.Symbols {
		baseCurrency, ok := constants.SYMBOL_TO_ASSET[symbol.BaseAsset]
		if !ok {
			continue
		}
		quoteCurrency, ok := constants.SYMBOL_TO_ASSET[symbol.QuoteAsset]
		if !ok {
			continue
		}
		security := models.Security{}
		security.Symbol = symbol.Symbol
		security.Underlying = &baseCurrency
		security.QuoteCurrency = &quoteCurrency
		security.Enabled = symbol.Status == "TRADING"
		security.Exchange = &constants.FBINANCE
		security.SecurityType = enum.SecurityType_CRYPTO_PERP
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name)
		security.MinPriceIncrement = 1. / math.Pow10(symbol.PricePrecision)
		security.RoundLot = 1. / math.Pow10(symbol.QuantityPrecision)
		security.IsInverse = false
		securities = append(securities, &security)
	}

	state.securities = make(map[uint64]*models.Security)
	state.symbolToSec = make(map[string]*models.Security)
	for _, s := range securities {
		state.securities[s.SecurityID] = s
		state.symbolToSec[s.Symbol] = s
	}

	context.Send(context.Parent(), &messages.SecurityList{
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: securities})

	return nil
}

func (state *Executor) OnSecurityListRequest(context actor.Context) error {
	// Get http request and the expected response
	msg := context.Message().(*messages.SecurityListRequest)
	securities := make([]*models.Security, len(state.securities))
	i := 0
	for _, v := range state.securities {
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

func (state *Executor) OnMarketDataRequest(context actor.Context) error {
	var snapshot *models.OBL2Snapshot
	msg := context.Message().(*messages.MarketDataRequest)
	response := &messages.MarketDataResponse{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    false,
	}
	if msg.Subscribe {
		response.RejectionReason = messages.SubscriptionNotSupported
		context.Respond(response)
		return nil
	}
	if msg.Instrument == nil || msg.Instrument.Symbol == nil {
		response.RejectionReason = messages.MissingInstrument
		context.Respond(response)
		return nil
	}
	symbol := msg.Instrument.Symbol.Value
	// Get http request and the expected response
	request, weight, err := fbinance.GetOrderBook(symbol, 1000)
	if err != nil {
		return err
	}

	if state.globalRateLimit.IsRateLimited() {
		response.RejectionReason = messages.RateLimitExceeded
		context.Respond(response)
		return nil
	}

	state.globalRateLimit.Request(weight)
	future := context.RequestFuture(state.queryRunner, &jobs.PerformQueryRequest{Request: request}, 10*time.Second)

	context.AwaitFuture(future, func(res interface{}, err error) {
		if err != nil {
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.HTTPError
			context.Respond(response)
			return
		}
		queryResponse := res.(*jobs.PerformQueryResponse)
		if queryResponse.StatusCode != 200 {
			if queryResponse.StatusCode >= 400 && queryResponse.StatusCode < 500 {
				err := fmt.Errorf(
					"http client error: %d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				state.logger.Info("http client error", log.Error(err))
				response.RejectionReason = messages.HTTPError
				context.Respond(response)
				return
			} else if queryResponse.StatusCode >= 500 {
				err := fmt.Errorf(
					"http server error: %d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				state.logger.Info("http client error", log.Error(err))
				response.RejectionReason = messages.HTTPError
				context.Respond(response)
				return
			}
			return
		}
		var obData fbinance.OrderBookData
		err = json.Unmarshal(queryResponse.Response, &obData)
		if err != nil {
			err = fmt.Errorf("error decoding query response: %v", err)
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.ExchangeAPIError
			context.Respond(response)
			return
		}
		if obData.Code != 0 {
			err = fmt.Errorf("error getting orderbook: %d %s", obData.Code, obData.Msg)
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.ExchangeAPIError
			context.Respond(response)
			return
		}

		bids, asks, err := obData.ToBidAsk()
		if err != nil {
			err = fmt.Errorf("error converting orderbook: %v", err)
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.ExchangeAPIError
			context.Respond(response)
			return
		}
		snapshot = &models.OBL2Snapshot{
			Bids:      bids,
			Asks:      asks,
			Timestamp: &types.Timestamp{Seconds: 0, Nanos: 0},
		}
		response.Success = true
		response.SnapshotL2 = snapshot
		response.SeqNum = obData.LastUpdateID
		context.Respond(response)
	})

	return nil
}

func (state *Executor) OnOrderStatusRequest(context actor.Context) error {
	msg := context.Message().(*messages.OrderStatusRequest)
	response := &messages.OrderList{
		RequestID: msg.RequestID,
		Success:   false,
	}
	filters := make(map[string]interface{})
	params := bitmex.NewGetOrderParams()
	if msg.Filter != nil {
		if msg.Filter.Instrument != nil {
			if msg.Filter.Instrument.Symbol != nil {
				params.SetSymbol(msg.Filter.Instrument.Symbol.Value)
			} else if msg.Filter.Instrument.SecurityID != nil {
				sec, ok := state.securities[msg.Filter.Instrument.SecurityID.Value]
				if !ok {
					response.RejectionReason = messages.UnknownSecurityID
					context.Respond(response)
					return nil
				}
				params.SetSymbol(sec.Symbol)
			}
		}
		if msg.Filter.OrderID != nil {
			filters["orderID"] = msg.Filter.OrderID.Value
		}
		if msg.Filter.ClientOrderID != nil {
			filters["clOrdID"] = msg.Filter.ClientOrderID.Value
		}
	}
	filters["open"] = true

	if len(filters) > 0 {
		params.SetFilters(filters)
	}

	request, weight, err := bitmex.GetOrder(msg.Account.Credentials, params)
	if err != nil {
		return err
	}

	if state.globalRateLimit.IsRateLimited() {
		response.RejectionReason = messages.RateLimitExceeded
		context.Respond(response)
		return nil
	}

	state.globalRateLimit.Request(weight)
	future := context.RequestFuture(state.queryRunner, &jobs.PerformQueryRequest{Request: request}, 10*time.Second)

	context.AwaitFuture(future, func(res interface{}, err error) {
		if err != nil {
			state.logger.Info("request error", log.Error(err))
			response.RejectionReason = messages.HTTPError
			context.Respond(response)
			return
		}
		queryResponse := res.(*jobs.PerformQueryResponse)
		if queryResponse.StatusCode != 200 {
			if queryResponse.StatusCode >= 400 && queryResponse.StatusCode < 500 {

				err := fmt.Errorf(
					"%d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				state.logger.Info("http error", log.Error(err))
				response.RejectionReason = messages.ExchangeAPIError
				context.Respond(response)
			} else if queryResponse.StatusCode >= 500 {

				err := fmt.Errorf(
					"%d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				state.logger.Info("http error", log.Error(err))
				response.RejectionReason = messages.ExchangeAPIError
				context.Respond(response)
			}
			return
		}
		var orders []bitmex.Order
		err = json.Unmarshal(queryResponse.Response, &orders)
		if err != nil {
			state.logger.Info("http error", log.Error(err))
			response.RejectionReason = messages.ExchangeAPIError
			context.Respond(response)
			return
		}

		var morders []*models.Order
		for _, o := range orders {
			sec, ok := state.symbolToSec[o.Symbol]
			if !ok {
				state.logger.Info("http error", log.Error(err))
				response.RejectionReason = messages.ExchangeAPIError
				context.Respond(response)
				return
			}
			ord := models.Order{
				OrderID: o.OrderID,
				Instrument: &models.Instrument{
					Exchange:   &constants.BITMEX,
					Symbol:     &types.StringValue{Value: o.Symbol},
					SecurityID: &types.UInt64Value{Value: sec.SecurityID},
				},
				LeavesQuantity: float64(o.LeavesQty),
				CumQuantity:    float64(o.CumQty),
			}

			if o.ClOrdID != nil {
				ord.ClientOrderID = *o.ClOrdID
			}

			switch o.OrdStatus {
			case "New":
				ord.OrderStatus = models.New
			case "Canceled":
				ord.OrderStatus = models.Canceled
			case "Filled":
				ord.OrderStatus = models.Filled
			default:
				fmt.Println("UNKNWOEN ORDER STATUS", o.OrdStatus)
			}

			switch bitmex.OrderType(o.OrdType) {
			case bitmex.LIMIT:
				ord.OrderType = models.Limit
			case bitmex.MARKET:
				ord.OrderType = models.Market
			case bitmex.STOP:
				ord.OrderType = models.Stop
			case bitmex.STOP_LIMIT:
				ord.OrderType = models.StopLimit
			case bitmex.LIMIT_IF_TOUCHED:
				ord.OrderType = models.LimitIfTouched
			case bitmex.MARKET_IF_TOUCHED:
				ord.OrderType = models.MarketIfTouched
			default:
				fmt.Println("UNKNOWN ORDER TYPE", o.OrdType)
			}

			switch bitmex.OrderSide(o.Side) {
			case bitmex.BUY_ORDER_SIDE:
				ord.Side = models.Buy
			case bitmex.SELL_ORDER_SIDE:
				ord.Side = models.Sell
			default:
				fmt.Println("UNKNOWN ORDER SIDE", o.Side)
			}

			switch bitmex.TimeInForce(o.TimeInForce) {
			case bitmex.TIF_DAY:
				ord.TimeInForce = models.Session
			case bitmex.GOOD_TILL_CANCEL:
				ord.TimeInForce = models.GoodTillCancel
			case bitmex.IMMEDIATE_OR_CANCEL:
				ord.TimeInForce = models.ImmediateOrCancel
			case bitmex.FILL_OR_KILL:
				ord.TimeInForce = models.FillOrKill
			default:
				fmt.Println("UNKNOWN TOF", o.TimeInForce)
			}

			ord.Price = &types.DoubleValue{Value: o.Price}

			morders = append(morders, &ord)
		}
		response.Success = true
		response.Orders = morders
		context.Respond(response)
	})

	return nil
}

func (state *Executor) OnPositionsRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnBalancesRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnNewOrderSingleRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnNewOrderBulkRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderReplaceRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderBulkReplaceRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderCancelRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderMassCancelRequest(context actor.Context) error {
	return nil
}
