package bitstamp

import (
	"encoding/json"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/enum"
	extypes "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/jobs"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/bitstamp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io/ioutil"
	"math"
	"net/http"
	"reflect"
	"strings"
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
type Executor struct {
	extypes.BaseExecutor
	client      *http.Client
	rateLimit   *exchanges.RateLimit
	queryRunner *actor.PID
	logger      *log.Logger
}

func NewExecutor() actor.Actor {
	return &Executor{
		client:      nil,
		rateLimit:   nil,
		queryRunner: nil,
		logger:      nil,
	}
}

func (state *Executor) Receive(context actor.Context) {
	extypes.ReceiveExecutor(state, context)
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
	state.rateLimit = exchanges.NewRateLimit(8000, 10*time.Minute)
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(state).String()))
	props := actor.PropsFromProducer(func() actor.Actor {
		return jobs.NewHTTPQuery(state.client)
	})
	state.queryRunner = context.Spawn(props)

	return state.UpdateSecurityList(context)
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, weight, err := bitstamp.GetTradingPairsInfo()
	if err != nil {
		return err
	}

	if state.rateLimit.IsRateLimited() {
		return fmt.Errorf("rate limited")
	}

	state.rateLimit.Request(weight)

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

	var tradingPairs []bitstamp.TradingPair
	err = json.Unmarshal(response, &tradingPairs)
	if err != nil {
		err = fmt.Errorf("error decoding query response: %v", err)
		return err
	}

	var securities []*models.Security
	for _, pair := range tradingPairs {
		security := models.Security{}
		if pair.Trading == "Enabled" {
			security.Status = models.InstrumentStatus_Trading
		} else {
			security.Status = models.InstrumentStatus_Disabled
		}
		baseName := strings.Split(pair.Name, "/")[0]
		quoteName := strings.Split(pair.Name, "/")[1]
		baseCurrency, ok := constants.GetAssetBySymbol(baseName)
		if !ok {
			continue
		}
		quoteCurrency, ok := constants.GetAssetBySymbol(quoteName)
		if !ok {
			continue
		}
		security.Symbol = pair.URLSymbol
		security.Underlying = baseCurrency
		security.QuoteCurrency = quoteCurrency
		security.Exchange = constants.BITSTAMP
		security.SecurityType = enum.SecurityType_CRYPTO_SPOT
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name, security.MaturityDate)
		security.RoundLot = &wrapperspb.DoubleValue{Value: 1. / math.Pow10(pair.BaseDecimals)}
		security.MinPriceIncrement = &wrapperspb.DoubleValue{Value: 1. / math.Pow10(pair.CounterDecimals)}
		securities = append(securities, &security)
	}

	state.SyncSecurities(securities, nil)

	context.Send(context.Parent(), &messages.SecurityList{
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: securities,
	})

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
	if msg.Instrument == nil || msg.Instrument.Symbol == nil {
		response.RejectionReason = messages.RejectionReason_MissingInstrument
		context.Respond(response)
		return nil
	}
	symbol := msg.Instrument.Symbol.Value

	if msg.Aggregation == models.OrderBookAggregation_L2 {
		var snapshot *models.OBL2Snapshot
		// Get http request and the expected response
		request, weight, err := bitstamp.GetOrderBook(
			symbol,
			bitstamp.OrderBookL2Group)
		if err != nil {
			return err
		}

		if state.rateLimit.IsRateLimited() {
			response.RejectionReason = messages.RejectionReason_IPRateLimitExceeded
			context.Respond(response)
			return nil
		}

		state.rateLimit.Request(weight)

		future := context.RequestFuture(state.queryRunner, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)

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

			var obData bitstamp.OrderBookL2
			err = json.Unmarshal(queryResponse.Response, &obData)
			if err != nil {
				err = fmt.Errorf("error decoding query response: %v", err)
				state.logger.Info("http client error", log.Error(err))
				response.RejectionReason = messages.RejectionReason_ExchangeAPIError
				context.Respond(response)
				return
			}

			bids, asks := obData.ToBidAsk()
			snapshot = &models.OBL2Snapshot{
				Bids:      bids,
				Asks:      asks,
				Timestamp: utils.MicroToTimestamp(obData.MicroTimestamp),
			}
			response.SnapshotL2 = snapshot
			response.SeqNum = obData.MicroTimestamp
			response.Success = true
			context.Respond(response)
		})
	} else {
		var snapshot *models.OBL3Snapshot
		// Get http request and the expected response
		request, weight, err := bitstamp.GetOrderBook(
			symbol,
			bitstamp.OrderBookL3Group)
		if err != nil {
			return err
		}

		if state.rateLimit.IsRateLimited() {
			response.RejectionReason = messages.RejectionReason_IPRateLimitExceeded
			context.Respond(response)
			return nil
		}

		state.rateLimit.Request(weight)

		future := context.RequestFuture(state.queryRunner, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)

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

			var obData bitstamp.OrderBookL3
			err = json.Unmarshal(queryResponse.Response, &obData)
			if err != nil {
				err = fmt.Errorf("error decoding query response: %v", err)
				state.logger.Info("http client error", log.Error(err))
				response.RejectionReason = messages.RejectionReason_ExchangeAPIError
				context.Respond(response)
				return
			}

			bids, asks := obData.ToBidAsk()
			snapshot = &models.OBL3Snapshot{
				Bids:      bids,
				Asks:      asks,
				Timestamp: utils.MicroToTimestamp(obData.MicroTimestamp),
			}
			response.SnapshotL3 = snapshot
			response.SeqNum = obData.MicroTimestamp
			response.Success = true
			context.Respond(response)
		})
	}

	return nil
}
