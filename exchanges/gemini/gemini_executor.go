package gemini

import (
	"encoding/json"
	"fmt"
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/log"
	"gitlab.com/alphaticks/alphac/enum"
	"gitlab.com/alphaticks/alphac/exchanges/interface"
	"gitlab.com/alphaticks/alphac/jobs"
	"gitlab.com/alphaticks/alphac/models"
	"gitlab.com/alphaticks/alphac/models/messages"
	"gitlab.com/alphaticks/alphac/utils"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/gemini"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type Executor struct {
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
	_interface.ExchangeExecutorReceive(state, context)
}

func (state *Executor) GetLogger() *log.Logger {
	return state.logger
}

func (state *Executor) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))

	state.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
	state.rateLimit = exchanges.NewRateLimit(3, time.Second)
	props := actor.PropsFromProducer(func() actor.Actor {
		return jobs.NewAPIQuery(state.client)
	})
	state.queryRunner = context.Spawn(props)

	return nil
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) OnSecurityListRequest(context actor.Context) error {
	// Get http request and the expected response
	msg := context.Message().(*messages.SecurityListRequest)
	request, weight, err := gemini.GetSymbols()
	if err != nil {
		return err
	}

	if state.rateLimit.IsRateLimited() {
		time.Sleep(state.rateLimit.DurationBeforeNextRequest(weight))
		return nil
	}

	// TODO Rate limit

	future := context.RequestFuture(state.queryRunner, &jobs.PerformQueryRequest{Request: request}, 10*time.Second)

	context.AwaitFuture(future, func(res interface{}, err error) {
		if err != nil {
			context.Respond(&messages.SecurityList{
				RequestID:  msg.RequestID,
				ResponseID: uint64(time.Now().UnixNano()),
				Error:      err.Error(),
				Securities: nil})
			return
		}
		queryResponse := res.(*jobs.PerformQueryResponse)
		if queryResponse.StatusCode != 200 {
			if queryResponse.StatusCode >= 400 && queryResponse.StatusCode < 500 {
				err := fmt.Errorf(
					"http client error: %d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				context.Respond(&messages.SecurityList{
					RequestID:  msg.RequestID,
					ResponseID: uint64(time.Now().UnixNano()),
					Error:      err.Error(),
					Securities: nil})
			} else if queryResponse.StatusCode >= 500 {
				err := fmt.Errorf(
					"http server error: %d %s",
					queryResponse.StatusCode,
					string(queryResponse.Response))
				context.Respond(&messages.SecurityList{
					RequestID:  msg.RequestID,
					ResponseID: uint64(time.Now().UnixNano()),
					Error:      err.Error(),
					Securities: nil})
			}
			return
		}
		var symbols []string
		err = json.Unmarshal(queryResponse.Response, &symbols)
		if err != nil {
			err = fmt.Errorf(
				"error unmarshaling response: %v",
				err)
			context.Respond(&messages.SecurityList{
				RequestID:  msg.RequestID,
				ResponseID: uint64(time.Now().UnixNano()),
				Error:      err.Error(),
				Securities: nil})
			return
		}

		var securities []*models.Security
		for _, symbol := range symbols {
			baseStr := strings.ToUpper(symbol[:3])
			quoteStr := strings.ToUpper(symbol[3:])
			baseCurrency, ok := constants.SYMBOL_TO_ASSET[baseStr]
			if !ok {
				state.logger.Info(fmt.Sprintf("unknown currency %s", baseStr))
				continue
			}
			quoteCurrency, ok := constants.SYMBOL_TO_ASSET[quoteStr]
			if !ok {
				state.logger.Info(fmt.Sprintf("unknown currency %s", quoteStr))
				continue
			}
			security := models.Security{}
			security.Symbol = symbol
			security.Underlying = &baseCurrency
			security.QuoteCurrency = &quoteCurrency
			security.Enabled = true
			security.Exchange = &constants.GEMINI
			security.SecurityType = enum.SecurityType_CRYPTO_SPOT
			security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name)
			if tickPrecision, ok := gemini.SYMBOL_TO_TICK_PRECISION[symbol]; ok {
				security.MinPriceIncrement = 1. / float64(tickPrecision)
			} else {
				state.logger.Info(fmt.Sprintf("unknown tick precision for %s", symbol))
				continue
			}
			if lotPrecision, ok := gemini.SYMBOL_TO_LOT_PRECISION[symbol]; ok {
				security.RoundLot = 1. / float64(lotPrecision)
			} else {
				state.logger.Info(fmt.Sprintf("unknown lot precision for %s", symbol))
				continue
			}

			securities = append(securities, &security)
		}
		context.Respond(&messages.SecurityList{
			RequestID:  msg.RequestID,
			ResponseID: uint64(time.Now().UnixNano()),
			Error:      "",
			Securities: securities})
	})

	return nil
}

func (state *Executor) OnMarketDataRequest(context actor.Context) error {
	return nil
}
