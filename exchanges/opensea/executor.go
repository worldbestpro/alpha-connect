package opensea

import (
	goContext "context"
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/types"
	gorderbook "gitlab.com/alphaticks/gorderbook/gorderbook.models"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges"
	"math/big"
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"time"

	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	models2 "gitlab.com/alphaticks/xchanger/models"
	xutils "gitlab.com/alphaticks/xchanger/utils"

	extype "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/jobs"
	opensea "gitlab.com/alphaticks/xchanger/exchanges/opensea"
)

type QueryRunner struct {
	pid       *actor.PID
	rateLimit *exchanges.RateLimit
}

type Executor struct {
	extype.BaseExecutor
	queryRunners     []*QueryRunner
	marketableAssets map[uint64]*models.MarketableAsset
	credentials      *models2.APICredentials
	dialerPool       *xutils.DialerPool
	logger           *log.Logger
	registry         registry.PublicRegistryClient
}

func NewExecutor(registry registry.PublicRegistryClient, dialerPool *xutils.DialerPool, credentials *models2.APICredentials) actor.Actor {
	return &Executor{
		dialerPool:  dialerPool,
		registry:    registry,
		credentials: credentials,
	}
}

func (state *Executor) getQueryRunner() *QueryRunner {
	sort.Slice(state.queryRunners, func(i, j int) bool {
		return rand.Uint64()%2 == 0
	})

	for _, q := range state.queryRunners {
		if !q.rateLimit.IsRateLimited() {
			return q
		}
	}
	return nil
}

func (state *Executor) Receive(context actor.Context) {
	extype.ReceiveExecutor(state, context)
}

func (state *Executor) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))

	dialers := state.dialerPool.GetDialers()
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
			rateLimit: exchanges.NewRateLimit(100, time.Minute),
		})
	}
	return state.UpdateMarketableAssetList(context)
}

func (state *Executor) UpdateMarketableAssetList(context actor.Context) error {
	assets := make([]*models.MarketableAsset, 0)
	reg := state.registry

	ctx, cancel := goContext.WithTimeout(goContext.Background(), 10*time.Second)
	defer cancel()
	//TODO add matic and solana protocols
	filter := registry.ProtocolAssetFilter{
		ProtocolId: []uint32{constants.ERC721.ID},
	}
	in := registry.ProtocolAssetsRequest{
		Filter: &filter,
	}
	res, err := reg.ProtocolAssets(ctx, &in)
	if err != nil {
		return fmt.Errorf("error updating protocol asset list: %v", err)
	}
	response := res.ProtocolAssets
	for _, protocolAsset := range response {
		if addr, ok := protocolAsset.Meta["address"]; !ok || len(addr) < 2 {
			state.logger.Warn("invalid protocol asset address")
			continue
		}
		_, ok := big.NewInt(1).SetString(protocolAsset.Meta["address"][2:], 16)
		if !ok {
			state.logger.Warn("invalid protocol asset address", log.Error(err))
			continue
		}
		as, ok := constants.GetAssetByID(protocolAsset.AssetId)
		if !ok {
			state.logger.Warn(fmt.Sprintf("error getting asset with id %d", protocolAsset.AssetId))
			continue
		}
		ch, ok := constants.GetChainByID(protocolAsset.ChainId)
		if !ok {
			state.logger.Warn(fmt.Sprintf("error getting chain with id %d", protocolAsset.ChainId))
			continue
		}
		assets = append(
			assets,
			&models.MarketableAsset{
				ProtocolAsset: &models.ProtocolAsset{
					ProtocolAssetID: protocolAsset.ProtocolAssetId,
					Protocol: &models2.Protocol{
						ID:   constants.ERC721.ID,
						Name: "ERC-721",
					},
					Asset: &models2.Asset{
						Name:   as.Name,
						Symbol: as.Symbol,
						ID:     as.ID,
					},
					Chain: &models2.Chain{
						ID:   ch.ID,
						Name: ch.Name,
						Type: ch.Type,
					},
					Meta: protocolAsset.Meta,
				},
				Market: &constants.OPENSEA,
			},
		)
	}
	state.marketableAssets = make(map[uint64]*models.MarketableAsset)
	for _, a := range assets {
		state.marketableAssets[a.ProtocolAsset.ProtocolAssetID] = a
	}

	context.Send(context.Parent(), &messages.MarketableAssetList{
		ResponseID:       uint64(time.Now().UnixNano()),
		MarketableAssets: assets,
		Success:          true,
	})

	return nil
}

func (state *Executor) OnMarketableAssetListRequest(context actor.Context) error {
	req := context.Message().(*messages.MarketableAssetListRequest)
	passets := make([]*models.MarketableAsset, len(state.marketableAssets))
	i := 0
	for _, v := range state.marketableAssets {
		passets[i] = v
		i += 1
	}
	context.Respond(&messages.MarketableAssetList{
		RequestID:        req.RequestID,
		ResponseID:       uint64(time.Now().UnixNano()),
		Success:          true,
		MarketableAssets: passets,
	})
	return nil
}

func (state *Executor) OnHistoricalSalesRequest(context actor.Context) error {
	req := context.Message().(*messages.HistoricalSalesRequest)
	msg := &messages.HistoricalSalesResponse{
		RequestID:  req.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    false,
	}

	if state.credentials.APIKey == "" {
		msg.RejectionReason = messages.UnsupportedRequest
		context.Respond(msg)
		return nil
	}
	var pAssets []*models.MarketableAsset
	for _, v := range state.marketableAssets {
		if v.ProtocolAsset.Asset.ID == req.AssetID {
			pAssets = append(pAssets, v)
		}
	}
	if pAssets == nil {
		msg.RejectionReason = messages.UnknownAsset
		context.Respond(msg)
		return nil
	}

	//TODO change code to handle multiple protocolAssets
	if len(pAssets) > 1 {
		msg.RejectionReason = messages.UnsupportedRequest
		context.Respond(msg)
		return nil
	}
	asset := pAssets[0]

	qr := state.getQueryRunner()
	if qr == nil {
		msg.RejectionReason = messages.RateLimitExceeded
		context.Respond(msg)
		return nil
	}

	params := opensea.NewGetEventsParams()
	add := asset.ProtocolAsset.Meta["address"]
	params.SetAssetContractAddress(add)
	params.SetEventType("successful")
	if req.To != nil {
		params.SetOccurredBefore(uint64(req.To.Seconds))
	}
	if req.From != nil {
		params.SetOccurredAfter(uint64(req.From.Seconds))
	}
	r, weight, err := opensea.GetEvents(params, state.credentials.APIKey)
	if err != nil {
		msg.RejectionReason = messages.UnsupportedOrderCharacteristic
		context.Respond(msg)
		return nil
	}
	qr.rateLimit.Request(weight)

	//Global variables
	cursor := ""
	done := false
	var sales []*models.Sale
	sender := context.Sender() //keep copy of sender

	var processFuture func(res interface{}, err error)
	processFuture = func(res interface{}, err error) {
		if err != nil {
			msg.RejectionReason = messages.HTTPError
			context.Respond(msg)
			return
		}

		resp := res.(*jobs.PerformQueryResponse)
		if resp.StatusCode != 200 {
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				err := fmt.Errorf("%d %s", resp.StatusCode, string(resp.Response))
				state.logger.Warn("http client error", log.Error(err))
				msg.RejectionReason = messages.HTTPError
				context.Respond(msg)
				return
			} else if resp.StatusCode >= 500 {
				err := fmt.Errorf("%d %s", resp.StatusCode, string(resp.Response))
				state.logger.Warn("http server error", log.Error(err))
				msg.RejectionReason = messages.HTTPError
				context.Respond(msg)
				return
			}
			return
		}

		var events opensea.EventsResponse
		if err := json.Unmarshal(resp.Response, &events); err != nil {
			msg.RejectionReason = messages.ExchangeAPIError
			context.Respond(msg)
			return
		}
		for _, e := range events.AssetEvents {
			var from [20]byte
			var to [20]byte
			var tokenID [32]byte
			var price [32]byte
			f, ok := big.NewInt(1).SetString(e.Transaction.FromAccount.Address[2:], 16)
			if !ok {
				state.logger.Warn("incorrect address format", log.String("address", e.Transaction.FromAccount.Address))
				continue
			}
			t, ok := big.NewInt(1).SetString(e.Transaction.ToAccount.Address[2:], 16)
			if !ok {
				state.logger.Warn("incorrect address format", log.String("address", e.Transaction.ToAccount.Address))
				continue
			}
			token, ok := big.NewInt(1).SetString(e.Asset.TokenId, 10)
			if !ok {
				state.logger.Warn("incorrect tokenID format", log.String("tokenID", e.Asset.TokenId))
				continue
			}
			i, err := strconv.ParseInt(e.Transaction.BlockNumber, 10, 64)
			if err != nil {
				state.logger.Warn("incorrect block number format", log.String("block number", e.Transaction.BlockNumber))
				continue
			}
			p, ok := big.NewInt(1).SetString(e.TotalPrice, 10)
			if !ok {
				state.logger.Warn("incorrect price format", log.String("price", e.TotalPrice))
				continue
			}
			tim, err := time.Parse("2006-01-02T15:04:05", e.Transaction.Timestamp)
			if err != nil {
				state.logger.Warn("incorrect timestamp format", log.String("ts", e.Transaction.Timestamp))
				continue
			}
			f.FillBytes(from[:])
			t.FillBytes(to[:])
			token.FillBytes(tokenID[:])
			p.FillBytes(price[:])
			sales = append(sales, &models.Sale{
				Transfer: &gorderbook.AssetTransfer{
					From:    from[:],
					To:      to[:],
					TokenId: tokenID[:],
				},
				Block:     uint64(i),
				Price:     price[:],
				Timestamp: &types.Timestamp{Seconds: tim.Unix()},
			})
		}
		cursor = events.Next
		done = cursor == ""
		if !done {
			params.SetCursor(cursor)
			r, weight, err = opensea.GetEvents(params, state.credentials.APIKey)
			if err != nil {
				msg.RejectionReason = messages.UnsupportedOrderCharacteristic
				context.Respond(msg)
				return
			}
			qr.rateLimit.Request(weight)
			fut := context.RequestFuture(qr.pid, &jobs.PerformHTTPQueryRequest{Request: r}, 15*time.Second)
			context.AwaitFuture(fut, processFuture)
		} else {
			msg.Success = true
			msg.Sale = sales
			msg.SeqNum = uint64(time.Now().UnixNano())
			context.Send(sender, msg)
		}
	}
	future := context.RequestFuture(qr.pid, &jobs.PerformHTTPQueryRequest{Request: r}, 10*time.Minute)
	context.AwaitFuture(future, processFuture)
	return nil
}

func (state *Executor) Clean(context actor.Context) error {
	return nil
}

func (state *Executor) GetLogger() *log.Logger {
	return state.logger
}
