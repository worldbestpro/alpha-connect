package bybits

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
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"gitlab.com/alphaticks/xchanger/exchanges/bybits"
	xutils "gitlab.com/alphaticks/xchanger/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"time"
)

type QueryRunner struct {
	pid       *actor.PID
	rateLimit *exchanges.RateLimit
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

func (state *Executor) getQueryRunner() *QueryRunner {
	sort.Slice(state.queryRunners, func(i, j int) bool {
		return rand.Uint64()%2 == 0
	})

	var qr *QueryRunner
	for _, q := range state.queryRunners {
		if !q.rateLimit.IsRateLimited() {
			qr = q
			break
		}
	}

	return qr
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
			pid:       context.Spawn(props),
			rateLimit: exchanges.NewRateLimit(50, time.Second),
		})
	}

	if err := state.UpdateSecurityList(context); err != nil {
		state.logger.Warn("error updating security list: %v", log.Error(err))
	}

	return nil
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) UpdateSecurityList(context actor.Context) error {
	request, weight, err := bybits.GetSymbols()
	if err != nil {
		return err
	}

	var qr *QueryRunner
	for _, q := range state.queryRunners {
		if !q.rateLimit.IsRateLimited() {
			qr = q
			break
		}
	}

	if qr == nil {
		return fmt.Errorf("rate limited")
	}

	qr.rateLimit.Request(weight)

	future := context.RequestFuture(qr.pid, &jobs.PerformHTTPQueryRequest{Request: request}, 10*time.Second)
	res, err := future.Result()
	if err != nil {
		return fmt.Errorf("http client error: %v", err)
	}
	queryResponse := res.(*jobs.PerformQueryResponse)
	if queryResponse.StatusCode != 200 {
		if queryResponse.StatusCode >= 400 && queryResponse.StatusCode < 500 {
			return fmt.Errorf(
				"http client error: %d %s",
				queryResponse.StatusCode,
				string(queryResponse.Response))
		} else if queryResponse.StatusCode >= 500 {
			return fmt.Errorf(
				"http server error: %d %s",
				queryResponse.StatusCode,
				string(queryResponse.Response))
		}
	}

	var data bybits.SymbolsResponse
	err = json.Unmarshal(queryResponse.Response, &data)
	if err != nil {
		err = fmt.Errorf(
			"error unmarshaling response: %v",
			err)
		return err
	}
	if data.RetCode != 0 {
		err = fmt.Errorf(
			"got wrong return code: %s",
			data.RetMsg)
		return err
	}

	var securities []*models.Security
	for _, symbol := range data.Result {
		baseCurrency, ok := constants.GetAssetBySymbol(symbol.BaseCurrency)
		if !ok {
			//state.logger.Info(fmt.Sprintf("unknown currency %s", baseStr))
			continue
		}
		quoteCurrency, ok := constants.GetAssetBySymbol(symbol.QuoteCurrency)
		if !ok {
			//state.logger.Info(fmt.Sprintf("unknown currency %s", baseStr))
			continue
		}
		security := models.Security{}
		security.Symbol = symbol.Name
		security.Underlying = baseCurrency
		security.QuoteCurrency = quoteCurrency
		security.Status = models.InstrumentStatus_Trading
		security.Exchange = constants.BYBITS
		security.SecurityType = enum.SecurityType_CRYPTO_SPOT
		security.SecurityID = utils.SecurityID(security.SecurityType, security.Symbol, security.Exchange.Name, security.MaturityDate)
		security.MinPriceIncrement = &wrapperspb.DoubleValue{Value: symbol.MinPricePrecision}
		security.RoundLot = &wrapperspb.DoubleValue{Value: symbol.BasePrecision}
		securities = append(securities, &security)
	}

	state.SyncSecurities(securities, nil)

	context.Send(context.Parent(), &messages.SecurityList{
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: securities})
	return nil
}
