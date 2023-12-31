package bitfinex

import (
	"encoding/json"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/enum"
	extypes "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/jobs"
	models "gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	gmodels "gitlab.com/alphaticks/gorderbook/gorderbook.models"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/bitfinex"
	xutils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Execute api calls
// Contains rate limit
// Spawn a query actor for each request
// and pipe its result back

// 429 rate limit
// 418 IP ban

type QueryRunner struct {
	pid              *actor.PID
	obRateLimit      *exchanges.RateLimit
	symbolsRateLimit *exchanges.RateLimit
}

type Executor struct {
	extypes.BaseExecutor
	queryRunners []*QueryRunner
	logger       *log.Logger
}

func NewExecutor(dialerPool *xutils.DialerPool, registry registry.StaticClient) actor.Actor {
	e := &Executor{}
	e.DialerPool = dialerPool
	e.Registry = registry
	return e
}

func (state *Executor) Receive(context actor.Context) {
	extypes.ReceiveExecutor(state, context)
}

func (state *Executor) GetLogger() *log.Logger {
	return state.logger
}

func (state *Executor) getQueryRunner(method string) *QueryRunner {
	sort.Slice(state.queryRunners, func(i, j int) bool {
		return rand.Uint64()%2 == 0
	})

	var qr *QueryRunner
out:
	for _, q := range state.queryRunners {
		switch method {
		case "OB":
			if !q.obRateLimit.IsRateLimited() {
				qr = q
				break out
			}
		case "SYMBOL":
			if !q.symbolsRateLimit.IsRateLimited() {
				qr = q
				break out
			}
		}
	}
	return qr
}

func (state *Executor) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(state).String()))

	dialers := state.DialerPool.GetDialers()
	for _, dialer := range dialers {
		client := &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 10 * time.Second,
				DialContext:         dialer.DialContext,
			},
			Timeout: 10 * time.Second,
		}
		props := actor.PropsFromProducer(func() actor.Actor {
			return jobs.NewHTTPQuery(client)
		})
		state.queryRunners = append(state.queryRunners, &QueryRunner{
			pid:              context.Spawn(props),
			obRateLimit:      exchanges.NewRateLimit(30, time.Minute),
			symbolsRateLimit: exchanges.NewRateLimit(10, time.Minute),
		})
	}

	if err := state.UpdateSecurityList(context); err != nil {
		state.logger.Info("error updating security list: %v", log.Error(err))
	}
	return nil
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, weight, err := bitfinex.GetSymbolsDetails()
	if err != nil {
		return err
	}

	qr := state.getQueryRunner("SYMBOL")

	if qr == nil {
		return fmt.Errorf("rate limited")
	}

	qr.symbolsRateLimit.Request(weight)

	future := context.RequestFuture(qr.pid, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)

	res, err := future.Result()
	if err != nil {
		return err
	}
	resp := res.(*jobs.PerformQueryResponse)

	if resp.StatusCode != 200 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			err := fmt.Errorf(
				"http client error: %d %s",
				resp.StatusCode,
				string(resp.Response))
			return err
		} else if resp.StatusCode >= 500 {
			err := fmt.Errorf(
				"http server error: %d %s",
				resp.StatusCode,
				string(resp.Response))
			return err
		} else {
			err := fmt.Errorf("%d %s",
				resp.StatusCode,
				string(resp.Response))
			return err
		}
	}
	var symbolDetails []bitfinex.SymbolDetail
	err = json.Unmarshal(resp.Response, &symbolDetails)
	if err != nil {
		err = fmt.Errorf("error decoding query response: %v", err)
		return err
	}

	var securities []*models.Security
	for _, symbol := range symbolDetails {
		security := models.Security{}
		if len(symbol.Pair) == 6 {
			symbolStr := strings.ToUpper(symbol.Pair[:3])
			if sym, ok := bitfinex.BITFINEX_SYMBOL_TO_GLOBAL_SYMBOL[symbolStr]; ok {
				symbolStr = sym
			}
			baseCurrency, ok := constants.GetAssetBySymbol(symbolStr)
			if !ok {
				//fmt.Println("UNKNOWN BASE", symbolStr)
				continue
			}
			symbolStr = strings.ToUpper(symbol.Pair[3:])
			if sym, ok := bitfinex.BITFINEX_SYMBOL_TO_GLOBAL_SYMBOL[symbolStr]; ok {
				symbolStr = sym
			}
			quoteCurrency, ok := constants.GetAssetBySymbol(symbolStr)
			if !ok {
				continue
			}
			security.Underlying = baseCurrency
			security.QuoteCurrency = quoteCurrency
			security.SecurityType = enum.SecurityType_CRYPTO_SPOT
		} else {
			splits := strings.Split(symbol.Pair, ":")
			if len(splits) != 2 {
				continue
			}
			base := splits[0]
			quote := splits[1]
			if quote[len(quote)-2:] == "f0" {
				base = base[:len(base)-2]
				quote = quote[:len(quote)-2]
				security.SecurityType = enum.SecurityType_CRYPTO_PERP
				security.IsInverse = false
			} else {
				security.SecurityType = enum.SecurityType_CRYPTO_SPOT
			}
			symbolStr := strings.ToUpper(base)
			if sym, ok := bitfinex.BITFINEX_SYMBOL_TO_GLOBAL_SYMBOL[symbolStr]; ok {
				symbolStr = sym
			}
			baseCurrency, ok := constants.GetAssetBySymbol(symbolStr)
			if !ok {
				continue
			}
			symbolStr = strings.ToUpper(quote)
			if sym, ok := bitfinex.BITFINEX_SYMBOL_TO_GLOBAL_SYMBOL[symbolStr]; ok {
				symbolStr = sym
			}
			quoteCurrency, ok := constants.GetAssetBySymbol(symbolStr)
			if !ok {
				continue
			}
			security.Underlying = baseCurrency
			security.QuoteCurrency = quoteCurrency
		}

		security.Status = models.InstrumentStatus_Trading
		security.Symbol = symbol.Pair
		security.Exchange = constants.BITFINEX
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name, security.MaturityDate)
		security.RoundLot = &wrapperspb.DoubleValue{Value: 1. / 100000000.}
		securities = append(securities, &security)
	}

	state.SyncSecurities(securities, nil)

	context.Send(context.Parent(), &messages.SecurityList{
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: securities})

	return nil
}

func (state *Executor) OnMarketDataRequest(context actor.Context) error {
	msg := context.Message().(*messages.MarketDataRequest)
	response := &messages.MarketDataResponse{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    false,
	}
	if msg.Subscribe {
		response.RejectionReason = messages.RejectionReason_UnsupportedSubscription
		context.Respond(response)
		return nil
	}
	if msg.Instrument == nil {
		response.RejectionReason = messages.RejectionReason_MissingInstrument
		context.Respond(response)
		return nil
	}
	symbol := msg.Instrument.Symbol.Value
	request, weight, err := bitfinex.GetOrderBook(symbol, 2500, 2500)
	if err != nil {
		return err
	}

	qr := state.getQueryRunner("OB")

	if qr == nil {
		response.RejectionReason = messages.RejectionReason_IPRateLimitExceeded
		context.Respond(response)
		return nil
	}
	qr.obRateLimit.Request(weight)

	future := context.RequestFuture(qr.pid, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)
	context.ReenterAfter(future, func(res interface{}, err error) {
		if err != nil {
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.RejectionReason_HTTPError
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
				response.RejectionReason = messages.RejectionReason_HTTPError
				context.Respond(response)
				return
			} else if queryResponse.StatusCode >= 500 {
				err := fmt.Errorf(
					"http server error: %d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				state.logger.Info("http client error", log.Error(err))
				response.RejectionReason = messages.RejectionReason_HTTPError
				context.Respond(response)
				return
			}
			return
		}

		var obData bitfinex.OrderBookData
		err = json.Unmarshal(queryResponse.Response, &obData)
		if err != nil {
			err = fmt.Errorf("error decoding query response: %v", err)
			state.logger.Info("http client error", log.Error(err))
			response.RejectionReason = messages.RejectionReason_ExchangeAPIError
			context.Respond(response)
			return
		}
		var bids []*gmodels.OrderBookLevel
		var asks []*gmodels.OrderBookLevel
		// TS is float in seconds, * 1000 + rounding to get millisecond
		var ts uint64 = 0
		for _, bid := range obData.Bids {
			if uint64(bid.Timestamp*1000) > ts {
				ts = uint64(bid.Timestamp * 1000)
			}
			bids = append(bids, &gmodels.OrderBookLevel{
				Price:    bid.Price,
				Quantity: bid.Amount,
				Bid:      true,
			})
		}
		for _, ask := range obData.Asks {
			if uint64(ask.Timestamp*1000) > ts {
				ts = uint64(ask.Timestamp * 1000)
			}
			asks = append(asks, &gmodels.OrderBookLevel{
				Price:    ask.Price,
				Quantity: ask.Amount,
				Bid:      false,
			})
		}

		snapshot := &models.OBL2Snapshot{
			Bids:      bids,
			Asks:      asks,
			Timestamp: utils.MilliToTimestamp(ts),
		}
		response.SnapshotL2 = snapshot
		response.SeqNum = ts
		response.Success = true
		context.Respond(response)
	})
	return nil
}
