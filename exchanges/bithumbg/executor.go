package bithumbg

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/enum"
	extypes "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/jobs"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	gmodels "gitlab.com/alphaticks/gorderbook/gorderbook.models"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/bithumbg"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io/ioutil"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"
)

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
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(state).String()))

	state.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
	// TODO
	state.rateLimit = exchanges.NewRateLimit(10, time.Second)
	props := actor.PropsFromProducer(func() actor.Actor {
		return jobs.NewHTTPQuery(state.client)
	})
	state.queryRunner = context.Spawn(props)
	return state.UpdateSecurityList(context)
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, weight, err := bithumbg.GetSpotConfig()
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

	var res bithumbg.Response
	if err := json.Unmarshal(response, &res); err != nil {
		err = fmt.Errorf(
			"error unmarshaling response: %v",
			err)
		return err
	}

	var config bithumbg.Config
	if err := json.Unmarshal(res.Data, &config); err != nil {
		err = fmt.Errorf(
			"error unmarshaling config: %v",
			err)
		return err
	}

	var securities []*models.Security
	for _, symbol := range config.Pairs {
		splits := strings.Split(symbol.Symbol, "-")
		baseStr := strings.ToUpper(splits[0])
		quoteStr := strings.ToUpper(splits[1])

		baseCurrency, ok := constants.GetAssetBySymbol(baseStr)
		if !ok {
			//state.logger.Info(fmt.Sprintf("unknown currency %s", baseStr))
			continue
		}
		quoteCurrency, ok := constants.GetAssetBySymbol(quoteStr)
		if !ok {
			//state.logger.Info(fmt.Sprintf("unknown currency %s", quoteStr))
			continue
		}
		security := models.Security{}
		security.Symbol = symbol.Symbol
		security.Underlying = baseCurrency
		security.QuoteCurrency = quoteCurrency
		security.Status = models.InstrumentStatus_Trading
		security.Exchange = constants.BITHUMBG
		security.SecurityType = enum.SecurityType_CRYPTO_SPOT
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name, security.MaturityDate)
		security.IsInverse = false
		security.MinPriceIncrement = &wrapperspb.DoubleValue{Value: 1. / math.Pow(10, float64(symbol.Accuracy[0]))}
		security.RoundLot = &wrapperspb.DoubleValue{Value: 1. / math.Pow(10, float64(symbol.Accuracy[1]))}
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
	if state.rateLimit.IsRateLimited() {
		response.RejectionReason = messages.RejectionReason_IPRateLimitExceeded
		context.Respond(response)
		return nil
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
	var symbol = ""
	if msg.Instrument.Symbol != nil {
		symbol = msg.Instrument.Symbol.Value
	} else if msg.Instrument.SecurityID != nil {
		sec := state.IDToSecurity(msg.Instrument.SecurityID.Value)
		if sec == nil {
			response.RejectionReason = messages.RejectionReason_UnknownSecurityID
			context.Respond(response)
			return nil
		}
		symbol = sec.Symbol
	}
	if symbol == "" {
		response.RejectionReason = messages.RejectionReason_UnknownSymbol
		context.Respond(response)
		return nil
	}

	if msg.Aggregation == models.OrderBookAggregation_L2 {
		var snapshot *models.OBL2Snapshot
		// Get http request and the expected response
		request, weight, err := bithumbg.GetSpotOrderBook(symbol)
		if err != nil {
			return err
		}

		state.rateLimit.Request(weight)
		future := context.RequestFuture(state.queryRunner, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)

		context.ReenterAfter(future, func(res interface{}, err error) {
			if err != nil {
				state.logger.Warn("http client error", log.Error(err))
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
					state.logger.Warn("http client error", log.Error(err))
					response.RejectionReason = messages.RejectionReason_HTTPError
					context.Respond(response)
				} else if queryResponse.StatusCode >= 500 {
					err := fmt.Errorf(
						"http server error: %d %s",
						queryResponse.StatusCode,
						string(queryResponse.Response))
					state.logger.Warn("http client error", log.Error(err))
					response.RejectionReason = messages.RejectionReason_HTTPError
					context.Respond(response)
				}
				return
			}
			var bresponse bithumbg.Response
			if err := json.Unmarshal(queryResponse.Response, &bresponse); err != nil {
				state.logger.Warn("error decoding query response", log.Error(err))
				response.RejectionReason = messages.RejectionReason_HTTPError
				context.Respond(response)
				return
			}
			if bresponse.Code != "0" {
				state.logger.Warn("error getting order book data", log.Error(errors.New(bresponse.Msg)))
				response.RejectionReason = messages.RejectionReason_HTTPError
				context.Respond(response)
				return
			}
			var obData bithumbg.OrderBook
			if err := json.Unmarshal(bresponse.Data, &obData); err != nil {
				state.logger.Warn("error decoding query response", log.Error(err))
				response.RejectionReason = messages.RejectionReason_HTTPError
				context.Respond(response)
				return
			}

			bidst, askst := obData.ToBidAsk()
			var bids, asks []*gmodels.OrderBookLevel
			for _, b := range bidst {
				if b.Quantity == 0. {
					continue
				}
				bids = append(bids, b)
			}
			for _, a := range askst {
				if a.Quantity == 0. {
					continue
				}
				asks = append(asks, a)
			}
			snapshot = &models.OBL2Snapshot{
				Bids:      bids,
				Asks:      asks,
				Timestamp: utils.MilliToTimestamp(0),
			}
			response.SnapshotL2 = snapshot
			response.SeqNum = uint64(obData.Version)
			response.Success = true
			context.Respond(response)
		})

		return nil
	} else {

		return nil
	}
}
