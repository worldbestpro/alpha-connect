package bitz

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
	"gitlab.com/alphaticks/xchanger/exchanges/bitz"
	"io/ioutil"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type Executor struct {
	client      *http.Client
	securities  []*models.Security
	rateLimit   *exchanges.RateLimit
	queryRunner *actor.PID
	logger      *log.Logger
}

func NewExecutor() actor.Actor {
	return &Executor{
		client:    nil,
		rateLimit: nil,
		logger:    nil,
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
	//TODO state.httpRateLimit = exchanges.NewRateLimit()
	props := actor.PropsFromProducer(func() actor.Actor {
		return jobs.NewAPIQuery(state.client)
	})
	state.queryRunner = context.Spawn(props)

	return state.UpdateSecurityList(context)
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, _, err := bitz.GetSymbolList()
	if err != nil {
		return err
	}

	/* TODO
	if state.httpRateLimit.IsRateLimited() {
		time.Sleep(state.httpRateLimit.DurationBeforeNextRequest(weight))
	}
	*/

	// TODO state.httpRateLimit.Request(weight)
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

	var symbols bitz.SymbolList
	err = json.Unmarshal(response, &symbols)
	if err != nil {
		return err
	}

	if symbols.Status != 200 {
		err = fmt.Errorf("got incorrect status code: %d", symbols.Status)
		return err
	}

	var securities []*models.Security
	for _, pair := range symbols.Data {
		baseCurrency, ok := constants.SYMBOL_TO_ASSET[strings.ToUpper(pair.CoinFrom)]
		if !ok {
			//state.logger.Info("unknown symbol " + pair.BaseCurrency + " for instrument " + pair.InstrumentID)
			continue
		}
		quoteCurrency, ok := constants.SYMBOL_TO_ASSET[strings.ToUpper(pair.CoinTo)]
		if !ok {
			//state.logger.Info("unknown symbol " + pair.QuoteCurrency + " for instrument " + pair.InstrumentID)
			continue
		}

		security := models.Security{}
		security.Symbol = pair.Name
		security.Underlying = &baseCurrency
		security.QuoteCurrency = &quoteCurrency
		security.Enabled = true
		security.Exchange = &constants.BITZ
		security.SecurityType = enum.SecurityType_CRYPTO_SPOT
		security.RoundLot = 1. / math.Pow(10, float64(pair.NumberFloat))
		security.MinPriceIncrement = 1. / math.Pow(10, float64(pair.PriceFloat))
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name)
		securities = append(securities, &security)
	}
	state.securities = securities

	context.Send(context.Parent(), &messages.SecurityList{
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: state.securities})

	return nil
}

func (state *Executor) OnSecurityListRequest(context actor.Context) error {
	msg := context.Message().(*messages.SecurityListRequest)

	context.Respond(&messages.SecurityList{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: state.securities})

	return nil
}

func (state *Executor) OnMarketDataRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderStatusRequest(context actor.Context) error {
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

func (state *Executor) OnOrderCancelRequest(context actor.Context) error {
	return nil
}

func (state *Executor) OnOrderMassCancelRequest(context actor.Context) error {
	return nil
}
