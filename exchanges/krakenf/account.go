package krakenf

import (
	"fmt"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"gitlab.com/alphaticks/alpha-connect/account"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	"gitlab.com/alphaticks/xchanger"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges/krakenf"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gorm.io/gorm"
)

type checkSocket struct{}
type checkAccount struct{}
type refreshKey struct{}
type refreshMarkPrices struct{}

type AccountListener struct {
	account                 *account.Account
	readOnly                bool
	seqNum                  uint64
	krakenfExecutor         *actor.PID
	ws                      *krakenf.Websocket
	executorManager         *actor.PID
	logger                  *log.Logger
	registry                registry.StaticClient
	checkAccountTicker      *time.Ticker
	checkSocketTicker       *time.Ticker
	refreshKeyTicker        *time.Ticker
	refreshMarkPricesTicker *time.Ticker
	lastPingTime            time.Time
	securities              map[uint64]*models.Security
	symbolToSec             map[string]*models.Security
	client                  *http.Client
	db                      *gorm.DB
}

func NewAccountListenerProducer(account *account.Account, registry registry.StaticClient, db *gorm.DB, client *http.Client, strict bool) actor.Producer {
	return func() actor.Actor {
		return NewAccountListener(account, registry, db, client, strict)
	}
}

func NewAccountListener(account *account.Account, registry registry.StaticClient, db *gorm.DB, client *http.Client, readOnly bool) actor.Actor {
	return &AccountListener{
		account:         account,
		readOnly:        readOnly,
		registry:        registry,
		seqNum:          0,
		ws:              nil,
		executorManager: nil,
		logger:          nil,
		db:              db,
		client:          client,
	}
}

func (state *AccountListener) Receive(context actor.Context) {
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

	case *messages.AccountDataRequest:
		if err := state.OnAccountDataRequest(context); err != nil {
			state.logger.Error("error processing OnAccountDataRequest", log.Error(err))
			panic(err)
		}

	case *messages.PositionsRequest:
		if err := state.OnPositionsRequest(context); err != nil {
			state.logger.Error("error processing OnPositionListRequest", log.Error(err))
			panic(err)
		}

	case *messages.BalancesRequest:
		if err := state.OnBalancesRequest(context); err != nil {
			state.logger.Error("error processing OnBalancesRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderStatusRequest:
		if err := state.OnOrderStatusRequest(context); err != nil {
			state.logger.Error("error processing OnOrderStatusRequest", log.Error(err))
			panic(err)
		}

	case *messages.AccountInformationRequest:
		if err := state.OnAccountInformationRequest(context); err != nil {
			state.logger.Error("error processing OnAccountInformationRequest", log.Error(err))
			panic(err)
		}

	case *messages.AccountMovementRequest:
		if err := state.OnAccountMovementRequest(context); err != nil {
			state.logger.Error("error processing OnAccountMovementRequest", log.Error(err))
			panic(err)
		}

	case *messages.TradeCaptureReportRequest:
		if err := state.OnTradeCaptureReportRequest(context); err != nil {
			state.logger.Error("error processing OnTradeCaptureReportRequest", log.Error(err))
			panic(err)
		}

	case *messages.NewOrderSingleRequest:
		if err := state.OnNewOrderSingleRequest(context); err != nil {
			state.logger.Error("error processing OnNewOrderSingleRequest", log.Error(err))
			panic(err)
		}

	case *messages.NewOrderBulkRequest:
		if err := state.OnNewOrderBulkRequest(context); err != nil {
			state.logger.Error("error processing NewOrderBulkRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderReplaceRequest:
		if err := state.OnOrderReplaceRequest(context); err != nil {
			state.logger.Error("error processing OrderReplaceRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderBulkReplaceRequest:
		if err := state.OnBulkOrderReplaceRequest(context); err != nil {
			state.logger.Error("error processing BulkOrderReplaceRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderCancelRequest:
		if err := state.OnOrderCancelRequest(context); err != nil {
			state.logger.Error("error processing OnOrderCancelRequest", log.Error(err))
			panic(err)
		}

	case *messages.OrderMassCancelRequest:
		if err := state.OnOrderMassCancelRequest(context); err != nil {
			state.logger.Error("error processing OnOrderMassCancelRequest", log.Error(err))
			panic(err)
		}

	case *xchanger.WebsocketMessage:
		if err := state.onWebsocketMessage(context); err != nil {
			state.logger.Error("error processing onWebsocketMessage", log.Error(err))
			panic(err)
		}

	case *checkSocket:
		if err := state.checkSocket(context); err != nil {
			state.logger.Error("error checking socket", log.Error(err))
			panic(err)
		}

	case *checkAccount:
		if err := state.checkAccount(context); err != nil {
			state.logger.Error("error checking account", log.Error(err))
			panic(err)
		}

	case *refreshMarkPrices:
		if err := state.refreshMarkPrices(context); err != nil {
			state.logger.Error("error refreshing mark prices", log.Error(err))
			panic(err)
		}
	}
}

func (state *AccountListener) Initialize(context actor.Context) error {
	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))
	state.krakenfExecutor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.KRAKENF.Name+"_executor")
	if state.client == nil {
		state.client = &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1024,
				TLSHandshakeTimeout: 10 * time.Second,
			},
			Timeout: 10 * time.Second,
		}
	} else {
		fmt.Println("USING CUSTOM CLIENT")
	}
	// Request securities
	executor := actor.NewPID(context.ActorSystem().Address(), "executor")
	res, err := context.RequestFuture(executor, &messages.SecurityListRequest{}, 10*time.Second).Result()
	if err != nil {
		return fmt.Errorf("error getting securities: %v", err)
	}
	securityList, ok := res.(*messages.SecurityList)
	if !ok {
		return fmt.Errorf("was expecting *messages.SecurityList, got %s", reflect.TypeOf(res).String())
	}
	if !securityList.Success {
		return fmt.Errorf("error getting securities: %s", securityList.RejectionReason.String())
	}
	// TODO filtering should be done by the executor, when specifying exchange in the request
	var filteredSecurities []*models.Security
	for _, s := range securityList.Securities {
		if s.Exchange.ID == state.account.Exchange.ID {
			filteredSecurities = append(filteredSecurities, s)
		}
	}

	/*
		// Then fetch fees
		res, err = context.RequestFuture(state.krakenfExecutor, &messages.AccountInformationRequest{
			Account: state.account.Account,
		}, 10*time.Second).Result()

		if err != nil {
			return fmt.Errorf("error getting account information from executor: %v", err)
		}

		information, ok := res.(*messages.AccountInformationResponse)
		if !ok {
			return fmt.Errorf("was expecting AccountInformationResponse, got %s", reflect.TypeOf(res).String())
		}

		if !information.Success {
			return fmt.Errorf("error fetching account information: %s", information.RejectionReason.String())
		}
	*/

	state.securities = make(map[uint64]*models.Security)
	state.symbolToSec = make(map[string]*models.Security)
	for _, sec := range filteredSecurities {
		state.securities[sec.SecurityID] = sec
		state.symbolToSec[sec.Symbol] = sec
	}
	state.seqNum = 0

	if err := state.subscribeAccount(context); err != nil {
		return fmt.Errorf("error subscribing to account: %v", err)
	}

	checkAccountTicker := time.NewTicker(1 * time.Minute)
	state.checkAccountTicker = checkAccountTicker
	go func(pid *actor.PID) {
		for {
			select {
			case <-checkAccountTicker.C:
				context.Send(pid, &checkAccount{})
			case <-time.After(2 * time.Minute):
				if state.checkAccountTicker != checkAccountTicker {
					return
				}
			}
		}
	}(context.Self())

	checkSocketTicker := time.NewTicker(5 * time.Second)
	state.checkSocketTicker = checkSocketTicker
	go func(pid *actor.PID) {
		for {
			select {
			case <-checkSocketTicker.C:
				context.Send(pid, &checkSocket{})
			case <-time.After(10 * time.Second):
				if state.checkSocketTicker != checkSocketTicker {
					return
				}
			}
		}
	}(context.Self())

	refreshKeyTicker := time.NewTicker(30 * time.Minute)
	state.refreshKeyTicker = refreshKeyTicker
	go func(pid *actor.PID) {
		for {
			select {
			case <-refreshKeyTicker.C:
				context.Send(pid, &refreshKey{})
			case <-time.After(31 * time.Minute):
				if state.refreshKeyTicker != refreshKeyTicker {
					return
				}
			}
		}
	}(context.Self())

	refreshMarkPricesTicker := time.NewTicker(5 * time.Second)
	state.refreshMarkPricesTicker = refreshMarkPricesTicker
	go func(pid *actor.PID) {
		for {
			select {
			case <-refreshMarkPricesTicker.C:
				context.Send(pid, &refreshMarkPrices{})
			case <-time.After(31 * time.Second):
				if state.refreshMarkPricesTicker != refreshMarkPricesTicker {
					return
				}
			}
		}
	}(context.Self())

	return nil
}

// TODO
func (state *AccountListener) Clean(context actor.Context) error {
	if state.ws != nil {
		if err := state.ws.Disconnect(); err != nil {
			state.logger.Info("error disconnecting socket", log.Error(err))
		}
	}

	if state.checkAccountTicker != nil {
		state.checkAccountTicker.Stop()
		state.checkAccountTicker = nil
	}

	if state.checkSocketTicker != nil {
		state.checkSocketTicker.Stop()
		state.checkSocketTicker = nil
	}

	if state.refreshKeyTicker != nil {
		state.refreshKeyTicker.Stop()
		state.refreshKeyTicker = nil
	}

	if state.refreshMarkPricesTicker != nil {
		state.refreshMarkPricesTicker.Stop()
		state.refreshMarkPricesTicker = nil
	}

	if !state.readOnly {
		for _, sec := range state.securities {
			context.Request(state.krakenfExecutor, &messages.OrderMassCancelRequest{
				Account: state.account.Account,
				Filter: &messages.OrderFilter{
					Instrument: &models.Instrument{
						SecurityID: &wrapperspb.UInt64Value{Value: sec.SecurityID},
						Symbol:     &wrapperspb.StringValue{Value: sec.Symbol},
						Exchange:   sec.Exchange,
					},
				},
			})
		}
	}

	return nil
}

func (state *AccountListener) OnAccountDataRequest(context actor.Context) error {
	msg := context.Message().(*messages.AccountDataRequest)
	res := &messages.AccountDataResponse{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Securities: state.account.GetSecurities(),
		Orders:     state.account.GetOrders(nil),
		Positions:  state.account.GetPositions(),
		Balances:   state.account.GetBalances(),
		SeqNum:     state.seqNum,
	}

	makerFee := state.account.GetMakerFee()
	takerFee := state.account.GetTakerFee()
	if makerFee != nil {
		res.MakerFee = &wrapperspb.DoubleValue{Value: *makerFee}
	}
	if takerFee != nil {
		res.TakerFee = &wrapperspb.DoubleValue{Value: *takerFee}
	}

	context.Respond(res)

	return nil
}

func (state *AccountListener) OnPositionsRequest(context actor.Context) error {
	msg := context.Message().(*messages.PositionsRequest)
	// TODO FILTER
	positions := state.account.GetPositions()
	context.Respond(&messages.PositionList{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Positions:  positions,
	})
	return nil
}

func (state *AccountListener) OnBalancesRequest(context actor.Context) error {
	msg := context.Message().(*messages.BalancesRequest)
	// TODO FILTER
	balances := state.account.GetBalances()
	context.Respond(&messages.BalanceList{
		RequestID:  msg.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    true,
		Balances:   balances,
	})
	return nil
}

func (state *AccountListener) OnOrderStatusRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderStatusRequest)
	if req.Filter != nil && req.Filter.Instrument != nil {
		req.Filter.Instrument.Symbol.Value = strings.ToUpper(req.Filter.Instrument.Symbol.Value)
	}
	orders := state.account.GetOrders(req.Filter)

	context.Respond(&messages.OrderList{
		RequestID: req.RequestID,
		Success:   true,
		Orders:    orders,
	})
	return nil
}

func (state *AccountListener) OnAccountInformationRequest(context actor.Context) error {
	context.Forward(state.krakenfExecutor)
	return nil
}

func (state *AccountListener) OnAccountMovementRequest(context actor.Context) error {
	context.Forward(state.krakenfExecutor)
	return nil
}

func (state *AccountListener) OnTradeCaptureReportRequest(context actor.Context) error {
	context.Forward(state.krakenfExecutor)
	return nil
}

func (state *AccountListener) OnNewOrderSingleRequest(context actor.Context) error {
	req := context.Message().(*messages.NewOrderSingleRequest)
	req.Account = state.account.Account
	if req.Expire != nil && req.Expire.AsTime().Before(time.Now()) {
		context.Respond(&messages.NewOrderSingleResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: messages.RejectionReason_RequestExpired,
		})
		return nil
	}
	// Check order quantity
	order := &models.Order{
		OrderID:               "",
		ClientOrderID:         req.Order.ClientOrderID,
		Instrument:            req.Order.Instrument,
		OrderStatus:           models.OrderStatus_PendingNew,
		OrderType:             req.Order.OrderType,
		Side:                  req.Order.OrderSide,
		TimeInForce:           req.Order.TimeInForce,
		LeavesQuantity:        req.Order.Quantity,
		Price:                 req.Order.Price,
		CumQuantity:           0,
		ExecutionInstructions: req.Order.ExecutionInstructions,
		Tag:                   req.Order.Tag,
	}
	report, res := state.account.NewOrder(order)
	if res != nil {
		context.Respond(&messages.NewOrderSingleResponse{
			RequestID:       req.RequestID,
			Success:         false,
			RejectionReason: *res,
		})
	} else {
		sender := context.Sender()
		reqResponse := &messages.NewOrderSingleResponse{
			RequestID: req.RequestID,
			Success:   false,
		}

		// Ack, we are responsible for sending the response
		if req.ResponseType == messages.ResponseType_Ack {
			reqResponse.Success = true
			context.Send(sender, reqResponse)
		}

		if report != nil {
			report.SeqNum = state.seqNum + 1
			state.seqNum += 1
			context.Send(context.Parent(), report)

			if report.ExecutionType == messages.ExecutionType_PendingNew {
				fut := context.RequestFuture(state.krakenfExecutor, req, 10*time.Second)
				context.ReenterAfter(fut, func(res interface{}, err error) {
					if req.ResponseType == messages.ResponseType_Result {
						defer context.Send(sender, reqResponse)
					}

					if err != nil {
						report, err := state.account.RejectNewOrder(order.ClientOrderID, messages.RejectionReason_Other)
						if err != nil {
							panic(err)
						}
						if report != nil {
							report.SeqNum = state.seqNum + 1
							state.seqNum += 1
							context.Send(context.Parent(), report)
						}
						reqResponse.RejectionReason = messages.RejectionReason_Other
						return
					}

					response := res.(*messages.NewOrderSingleResponse)

					if response.Success {
						confirmReport, _ := state.account.ConfirmNewOrder(order.ClientOrderID, response.OrderID, nil)
						if confirmReport != nil {
							confirmReport.SeqNum = state.seqNum + 1
							state.seqNum += 1
							context.Send(context.Parent(), confirmReport)
						}
						switch response.OrderStatus {
						case models.OrderStatus_Expired:
							// Fill or kill or other.
							cancelReport, err := state.account.ConfirmExpiredOrder(response.OrderID)
							if err != nil {
								fmt.Println("ERROR CONFIRMING EXPIRED", err)
							}
							if cancelReport != nil {
								cancelReport.SeqNum = state.seqNum + 1
								state.seqNum += 1
								context.Send(context.Parent(), cancelReport)
							}
						case models.OrderStatus_Filled:
							filledReport, err := state.account.PendingFilled(response.OrderID)
							if err != nil {
								fmt.Println("ERROR PENDING FILLED", err)
							}
							if filledReport != nil {
								filledReport.SeqNum = state.seqNum + 1
								state.seqNum += 1
								context.Send(context.Parent(), filledReport)
							}
						}
						reqResponse.Success = true
						reqResponse.NetworkRtt = response.NetworkRtt
					} else {
						nReport, _ := state.account.RejectNewOrder(order.ClientOrderID, response.RejectionReason)
						if nReport != nil {
							nReport.SeqNum = state.seqNum + 1
							state.seqNum += 1
							context.Send(context.Parent(), nReport)
						}
						reqResponse.RejectionReason = response.RejectionReason
						reqResponse.RateLimitDelay = response.RateLimitDelay
					}
				})
			} else {
				if req.ResponseType == messages.ResponseType_Result {
					reqResponse.Success = true
					context.Send(sender, reqResponse)
				}
			}
		} else {
			if req.ResponseType == messages.ResponseType_Result {
				reqResponse.Success = true
				context.Send(sender, reqResponse)
			}
		}
	}

	return nil
}

func (state *AccountListener) OnNewOrderBulkRequest(context actor.Context) error {
	req := context.Message().(*messages.NewOrderBulkRequest)
	req.Account = state.account.Account
	reports := make([]*messages.ExecutionReport, 0, len(req.Orders))
	response := &messages.NewOrderBulkResponse{
		RequestID:  req.RequestID,
		ResponseID: uint64(time.Now().UnixNano()),
		Success:    false,
	}
	symbol := ""
	if req.Orders[0].Instrument != nil && req.Orders[0].Instrument.Symbol != nil {
		symbol = req.Orders[0].Instrument.Symbol.Value
	}
	for _, order := range req.Orders {
		if order.Instrument == nil || order.Instrument.Symbol == nil {
			response.RejectionReason = messages.RejectionReason_UnknownSymbol
			context.Respond(response)
			return nil
		} else if symbol != order.Instrument.Symbol.Value {
			response.RejectionReason = messages.RejectionReason_DifferentSymbols
			context.Respond(response)
			return nil
		}
	}
	for _, reqOrder := range req.Orders {
		order := &models.Order{
			OrderID:               "",
			ClientOrderID:         reqOrder.ClientOrderID,
			Instrument:            reqOrder.Instrument,
			OrderStatus:           models.OrderStatus_PendingNew,
			OrderType:             reqOrder.OrderType,
			Side:                  reqOrder.OrderSide,
			TimeInForce:           reqOrder.TimeInForce,
			LeavesQuantity:        reqOrder.Quantity,
			Price:                 reqOrder.Price,
			CumQuantity:           0,
			ExecutionInstructions: reqOrder.ExecutionInstructions,
			Tag:                   reqOrder.Tag,
		}
		report, res := state.account.NewOrder(order)
		if res != nil {
			// Cancel all new order up until now
			for _, r := range reports {
				_, err := state.account.RejectNewOrder(r.ClientOrderID.Value, messages.RejectionReason_Other)
				if err != nil {
					return err
				}
			}
			response.RejectionReason = *res
			context.Respond(response)
			return nil
		}
		if report != nil {
			reports = append(reports, report)
		}
	}

	response.Success = true
	context.Respond(response)

	for _, report := range reports {
		report.SeqNum = state.seqNum + 1
		state.seqNum += 1
		context.Send(context.Parent(), report)
	}
	fut := context.RequestFuture(state.krakenfExecutor, req, 10*time.Second)
	context.ReenterAfter(fut, func(res interface{}, err error) {
		if err != nil {
			for _, r := range reports {
				report, err := state.account.RejectNewOrder(r.ClientOrderID.Value, messages.RejectionReason_Other)
				if err != nil {
					panic(err)
				}

				if report != nil {
					report.SeqNum = state.seqNum + 1
					state.seqNum += 1
					context.Send(context.Parent(), report)
				}
			}
			context.Respond(&messages.NewOrderBulkResponse{
				RequestID:       req.RequestID,
				Success:         false,
				RejectionReason: messages.RejectionReason_Other,
			})
			return
		}
		response := res.(*messages.NewOrderBulkResponse)
		context.Respond(response)
		if response.Success {
			for i, r := range reports {
				report, err := state.account.ConfirmNewOrder(r.ClientOrderID.Value, response.OrderIDs[i], nil)
				if err != nil {
					panic(err)
				}

				if report != nil {
					report.SeqNum = state.seqNum + 1
					state.seqNum += 1
					context.Send(context.Parent(), report)
				}
			}
		} else {
			for _, r := range reports {
				report, err := state.account.RejectNewOrder(r.ClientOrderID.Value, messages.RejectionReason_Other)
				if err != nil {
					panic(err)
				}

				if report != nil {
					report.SeqNum = state.seqNum + 1
					state.seqNum += 1
					context.Send(context.Parent(), report)
				}
			}
		}
	})
	return nil
}

func (state *AccountListener) OnOrderReplaceRequest(context actor.Context) error {
	// TODO
	req := context.Message().(*messages.OrderReplaceRequest)
	var ID string
	if req.Update.OrigClientOrderID != nil {
		ID = req.Update.OrigClientOrderID.Value
	} else if req.Update.OrderID != nil {
		ID = req.Update.OrderID.Value
	}
	// forward to executor directly
	fut := context.RequestFuture(state.krakenfExecutor, req, 10*time.Second)
	fmt.Println("Editing ", ID)
	report, rej := state.account.ReplaceOrder(ID, req.Update.Price, req.Update.Quantity)
	if rej != nil {
		context.Respond(&messages.OrderReplaceResponse{
			RequestID:       req.RequestID,
			RejectionReason: *rej,
			Success:         false,
		})
	} else {
		sender := context.Sender()
		reqResponse := &messages.OrderReplaceResponse{
			RequestID: req.RequestID,
			Success:   false,
		}

		report.SeqNum = state.seqNum + 1
		state.seqNum += 1
		context.Send(context.Parent(), report)
		if report.ExecutionType == messages.ExecutionType_PendingReplace {
			context.ReenterAfter(fut, func(res interface{}, err error) {
				defer context.Send(sender, reqResponse)
				if err != nil {
					fmt.Println("REJECT EDIT", ID)
					report, err := state.account.RejectReplaceOrder(ID, messages.RejectionReason_Other)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					reqResponse.RejectionReason = messages.RejectionReason_Other
					return
				}
				response := res.(*messages.OrderReplaceResponse)

				if response.Success {
					report, err := state.account.ConfirmReplaceOrder(ID, response.OrderID)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					reqResponse.OrderID = response.OrderID
					reqResponse.Success = true
				} else {
					fmt.Println("REJECT EDIT", ID)
					report, err := state.account.RejectReplaceOrder(ID, response.RejectionReason)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					if response.RejectionReason == messages.RejectionReason_UnknownOrder {
						report, err := state.account.PendingFilled(ID)
						if err != nil {
							panic(err)
						}
						if report != nil {
							report.SeqNum = state.seqNum + 1
							state.seqNum += 1
							context.Send(context.Parent(), report)
						}
					}
					reqResponse.RejectionReason = response.RejectionReason
				}
			})
		} else {
			// Result, we are responsible for sending the response
			fmt.Println("UNEXPECTED REPORT TYPE", report.ExecutionType.String())
			reqResponse.Success = true
			context.Send(sender, reqResponse)
		}
	}
	return nil
}

func (state *AccountListener) OnBulkOrderReplaceRequest(context actor.Context) error {
	//TODO
	return nil
}

func (state *AccountListener) OnOrderCancelRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderCancelRequest)
	var ID string
	if req.ClientOrderID != nil {
		ID = req.ClientOrderID.Value
	} else if req.OrderID != nil {
		ID = req.OrderID.Value
	}
	// forward to executor directly
	fut := context.RequestFuture(state.krakenfExecutor, req, 10*time.Second)
	fmt.Println("CANCELING ", ID)
	report, rej := state.account.CancelOrder(ID)
	if rej != nil {
		context.Respond(&messages.OrderCancelResponse{
			RequestID:       req.RequestID,
			RejectionReason: *rej,
			Success:         false,
		})
	} else {
		sender := context.Sender()
		reqResponse := &messages.OrderCancelResponse{
			RequestID: req.RequestID,
			Success:   false,
		}

		// Ack, we are responsible for sending the response
		if req.ResponseType == messages.ResponseType_Ack {
			reqResponse.Success = true
			context.Send(sender, reqResponse)
		}

		report.SeqNum = state.seqNum + 1
		state.seqNum += 1
		context.Send(context.Parent(), report)
		if report.ExecutionType == messages.ExecutionType_PendingCancel {
			context.ReenterAfter(fut, func(res interface{}, err error) {
				if req.ResponseType == messages.ResponseType_Result {
					defer context.Send(sender, reqResponse)
				}

				if err != nil {
					fmt.Println("REJECT CANCEL", ID)
					report, err := state.account.RejectCancelOrder(ID, messages.RejectionReason_Other)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					reqResponse.RejectionReason = messages.RejectionReason_Other
					return
				}
				response := res.(*messages.OrderCancelResponse)

				if response.Success {
					report, err := state.account.ConfirmCancelOrder(ID)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					reqResponse.NetworkRtt = response.NetworkRtt
					reqResponse.Success = true
				} else {
					fmt.Println("REJECT CANCEL", ID)
					report, err := state.account.RejectCancelOrder(ID, response.RejectionReason)
					if err != nil {
						panic(err)
					}
					if report != nil {
						report.SeqNum = state.seqNum + 1
						state.seqNum += 1
						context.Send(context.Parent(), report)
					}
					if response.RejectionReason == messages.RejectionReason_UnknownOrder {
						report, err := state.account.PendingFilled(ID)
						if err != nil {
							panic(err)
						}
						if report != nil {
							report.SeqNum = state.seqNum + 1
							state.seqNum += 1
							context.Send(context.Parent(), report)
						}
					}
					reqResponse.RejectionReason = response.RejectionReason
					reqResponse.RateLimitDelay = response.RateLimitDelay
				}
			})
		} else {
			// Result, we are responsible for sending the response
			fmt.Println("UNEXPECTED REPORT TYPE", report.ExecutionType.String())
			if req.ResponseType == messages.ResponseType_Result {
				reqResponse.Success = true
				context.Send(sender, reqResponse)
			}
		}
	}

	return nil
}

func (state *AccountListener) OnOrderMassCancelRequest(context actor.Context) error {
	req := context.Message().(*messages.OrderMassCancelRequest)
	orders := state.account.GetOrders(req.Filter)
	if len(orders) == 0 {
		context.Respond(&messages.OrderMassCancelResponse{
			RequestID: req.RequestID,
			Success:   true,
		})
		return nil
	}
	var reports []*messages.ExecutionReport
	for _, o := range orders {
		if o.OrderStatus != models.OrderStatus_New && o.OrderStatus != models.OrderStatus_PartiallyFilled {
			continue
		}
		report, res := state.account.CancelOrder(o.ClientOrderID)
		if res != nil {
			// Reject all cancel order up until now
			for _, r := range reports {
				_, err := state.account.RejectCancelOrder(r.ClientOrderID.Value, messages.RejectionReason_Other)
				if err != nil {
					return err
				}
			}

			context.Respond(&messages.OrderMassCancelResponse{
				RequestID:       req.RequestID,
				Success:         false,
				RejectionReason: *res,
			})

			return nil
		} else if report != nil {
			reports = append(reports, report)
		}
	}

	context.Respond(&messages.OrderMassCancelResponse{
		RequestID: req.RequestID,
		Success:   true,
	})

	for _, report := range reports {
		report.SeqNum = state.seqNum + 1
		state.seqNum += 1
		context.Send(context.Parent(), report)
	}
	fut := context.RequestFuture(state.krakenfExecutor, req, 10*time.Second)
	context.ReenterAfter(fut, func(res interface{}, err error) {
		if err != nil {
			for _, r := range reports {
				report, err := state.account.RejectCancelOrder(r.ClientOrderID.Value, messages.RejectionReason_Other)
				if err != nil {
					panic(err)
				}
				if report != nil {
					report.SeqNum = state.seqNum + 1
					state.seqNum += 1
					context.Send(context.Parent(), report)
				}
			}
			context.Respond(&messages.OrderMassCancelResponse{
				RequestID:       req.RequestID,
				Success:         false,
				RejectionReason: messages.RejectionReason_Other,
			})

			return
		}
		response := res.(*messages.OrderMassCancelResponse)
		context.Respond(response)
		if !response.Success {
			for _, r := range reports {
				report, err := state.account.RejectCancelOrder(r.ClientOrderID.Value, response.RejectionReason)
				if err != nil {
					panic(err)
				}
				if report != nil {
					report.SeqNum = state.seqNum + 1
					state.seqNum += 1
					context.Send(context.Parent(), report)
				}
			}
		}
	})

	return nil
}

func (state *AccountListener) onWebsocketMessage(context actor.Context) error {
	msg := context.Message().(*xchanger.WebsocketMessage)
	if state.ws == nil || msg.WSID != state.ws.ID {
		return nil
	}
	state.lastPingTime = time.Now()

	if msg.Message == nil {
		return fmt.Errorf("received nil message")
	}

	return nil
}

func (state *AccountListener) subscribeAccount(context actor.Context) error {
	if state.ws != nil {
		_ = state.ws.Disconnect()
	}

	ws := krakenf.NewWebsocket()
	// TODO Dialer
	if err := ws.Connect(nil); err != nil {
		return fmt.Errorf("error connecting to krakenf websocket: %v", err)
	}

	if err := ws.GetChallenge(state.account.ApiCredentials); err != nil {
		return fmt.Errorf("error getting challenge: %v", err)
	}
	if !ws.ReadMessage() {
		return fmt.Errorf("error reading WSInfo response")
	}
	if !ws.ReadMessage() {
		return fmt.Errorf("error reading challenge response")
	}

	challenge, ok := ws.Msg.Message.(krakenf.WSChallengeResponse)
	if !ok {
		return fmt.Errorf("was expecting challenge response, got %T", ws.Msg.Message)
	}
	if err := ws.PrivateSubscribe(state.account.ApiCredentials, challenge.Message, krakenf.WSBalanceFeed); err != nil {
		return fmt.Errorf("error subscribing to balance: %v", err)
	}
	if err := ws.PrivateSubscribe(state.account.ApiCredentials, challenge.Message, krakenf.WSOpenPositionsFeed); err != nil {
		return fmt.Errorf("error subscribing to open positions: %v", err)
	}
	if err := ws.PrivateSubscribe(state.account.ApiCredentials, challenge.Message, krakenf.WSFillsFeed); err != nil {
		return fmt.Errorf("error subscribing to fills: %v", err)
	}
	if err := ws.PrivateSubscribe(state.account.ApiCredentials, challenge.Message, krakenf.WSOpenOrdersVerboseFeed); err != nil {
		return fmt.Errorf("error subscribing to open orders: %v", err)
	}

	// Get balances, positions, orders
	var balances *krakenf.WSBalanceSnapshot
	var positions *krakenf.WSOpenPositions
	var orders *krakenf.WSOpenOrdersVerboseSnapshot

	ready := false
	for !ready {
		if !ws.ReadMessage() {
			return fmt.Errorf("error reading ws message")
		}
		switch msg := ws.Msg.Message.(type) {
		case krakenf.WSBalanceSnapshot:
			balances = &msg
		case krakenf.WSOpenPositions:
			positions = &msg
		case krakenf.WSOpenOrdersVerboseSnapshot:
			orders = &msg
		default:
			context.Send(context.Self(), ws.Msg)
		}
		ready = balances != nil && positions != nil && orders != nil
	}

	morders := make([]*models.Order, len(orders.Orders))
	for i, o := range orders.Orders {
		mo := WSOrderToModel(o)
		sec, ok := state.symbolToSec[strings.ToLower(o.Instrument)]
		if !ok {
			fmt.Println(state.symbolToSec)
			return fmt.Errorf("got order for unknown symbol %s", o.Instrument)
		}
		mo.Instrument.SecurityID = wrapperspb.UInt64(sec.SecurityID)
		morders[i] = mo
	}
	mpositions := make([]*models.Position, len(positions.Positions))
	for i, p := range positions.Positions {
		mp := WSPositionToModel(p)
		sec, ok := state.symbolToSec[strings.ToLower(p.Instrument)]
		if !ok {
			fmt.Println(state.symbolToSec)
			return fmt.Errorf("got position for unknown symbol %s", p.Instrument)
		}
		mp.Instrument.SecurityID = wrapperspb.UInt64(sec.SecurityID)
		mpositions[i] = mp
	}
	var mbalances []*models.Balance
	for k, v := range balances.FlexFutures.Currencies {
		// Get asset from symbol
		asset := SymbolToAsset(k)
		if asset == nil {
			return fmt.Errorf("unknown asset %s", k)
		}
		mbalances = append(mbalances, &models.Balance{
			Asset:    asset,
			Quantity: v.Quantity,
		})
	}

	msecurities := make([]*models.Security, len(state.securities))
	i := 0
	for _, s := range state.securities {
		msecurities[i] = s
		i += 1
	}

	var mf, tf = 0., 0.
	if err := state.account.Sync(msecurities, morders, mpositions, mbalances, &mf, &tf); err != nil {
		return fmt.Errorf("error syncing account: %v", err)
	}

	go func(ws *krakenf.Websocket, pid *actor.PID) {
		for ws.ReadMessage() {
			context.Send(pid, ws.Msg)
		}
	}(ws, context.Self())
	state.ws = ws

	return nil
}

func (state *AccountListener) checkSocket(context actor.Context) error {

	if time.Since(state.lastPingTime) > 5*time.Second {
		_ = state.ws.Ping()
		state.lastPingTime = time.Now()
	}

	if state.ws.Err != nil || !state.ws.Connected {
		if state.ws.Err != nil {
			state.logger.Info("error on socket", log.Error(state.ws.Err))
		}
		if err := state.subscribeAccount(context); err != nil {
			return fmt.Errorf("error subscribing to account: %v", err)
		}
	}

	return nil
}

func (state *AccountListener) refreshMarkPrices(context actor.Context) error {
	future := context.RequestFuture(state.krakenfExecutor, &messages.PositionsRequest{
		RequestID: 0,
		Account:   state.account.Account,
	}, 10*time.Second)
	context.ReenterAfter(future, func(res interface{}, err error) {
		if err != nil {
			state.logger.Error("error updating mark price", log.Error(err))
		}
		pos := res.(*messages.PositionList)
		if !pos.Success {
			state.logger.Error("error updating mark price", log.String("rejection", pos.RejectionReason.String()))
		}
		for _, p := range pos.Positions {
			if p.MarkPrice != nil {
				state.account.UpdateMarkPrice(p.Instrument.SecurityID.Value, p.MarkPrice.Value)
			}
			if p.MaxNotionalValue != nil {
				state.account.UpdateMaxNotionalValue(p.Instrument.SecurityID.Value, p.MaxNotionalValue.Value)
			}
		}
	})
	return nil
}

func (state *AccountListener) checkAccount(context actor.Context) error {
	state.account.CleanOrders()

	pos1 := state.account.GetPositions()
	// Fetch positions
	res, err := context.RequestFuture(state.krakenfExecutor, &messages.PositionsRequest{
		Instrument: nil,
		Account:    state.account.Account,
	}, 10*time.Second).Result()

	if err != nil {
		return fmt.Errorf("error getting positions from executor: %v", err)
	}

	positionList, ok := res.(*messages.PositionList)
	if !ok {
		return fmt.Errorf("was expecting PositionList, got %s", reflect.TypeOf(res).String())
	}

	if !positionList.Success {
		return fmt.Errorf("error getting positions: %s", positionList.RejectionReason.String())
	}

	var pos2 []*models.Position
	for _, p := range positionList.Positions {
		if p.Quantity != 0 {
			pos2 = append(pos2, p)
		}
	}
	if len(pos1) != len(pos2) {
		return fmt.Errorf("different number of positions: %d %d", len(pos1), len(pos2))
	}

	// sort
	sort.Slice(pos1, func(i, j int) bool {
		return pos1[i].Instrument.SecurityID.Value < pos1[j].Instrument.SecurityID.Value
	})
	sort.Slice(pos2, func(i, j int) bool {
		return pos2[i].Instrument.SecurityID.Value < pos2[j].Instrument.SecurityID.Value
	})

	for i := range pos1 {
		//lp := math.Ceil(1. / state.securities[pos1[i].Instrument.SecurityID.Value].RoundLot.Value)
		diff := math.Abs(pos1[i].Quantity-pos2[i].Quantity) / math.Abs(pos1[i].Quantity+pos2[i].Quantity)
		if diff > 0.01 {
			return fmt.Errorf("different position quantity: %f %f", pos1[i].Cost, pos2[i].Cost)
		}
		diff = math.Abs(pos1[i].Cost-pos2[i].Cost) / math.Abs(pos1[i].Cost+pos2[i].Cost)
		if diff > 0.01 {
			return fmt.Errorf("different position cost: %f %f", pos1[i].Cost, pos2[i].Cost)
		}
	}

	// Update
	for _, p := range pos2 {
		if p.MarkPrice != nil {
			state.account.UpdateMarkPrice(p.Instrument.SecurityID.Value, p.MarkPrice.Value)
		}
		if p.MaxNotionalValue != nil {
			state.account.UpdateMaxNotionalValue(p.Instrument.SecurityID.Value, p.MaxNotionalValue.Value)
		}
	}

	if err := state.account.CheckExpiration(); err != nil {
		return fmt.Errorf("error checking expired orders: %v", err)
	}

	// Fetch balances
	res, err = context.RequestFuture(state.krakenfExecutor, &messages.BalancesRequest{
		Account: state.account.Account,
	}, 10*time.Second).Result()

	if err != nil {
		return fmt.Errorf("error getting balances from executor: %v", err)
	}

	balanceList, ok := res.(*messages.BalanceList)
	if !ok {
		return fmt.Errorf("was expecting BalanceList, got %s", reflect.TypeOf(res).String())
	}

	if !balanceList.Success {
		return fmt.Errorf("error getting balances: %s", balanceList.RejectionReason.String())
	}

	balanceMap1 := make(map[uint32]float64)
	balanceMap2 := make(map[uint32]float64)
	for _, b := range balanceList.Balances {
		balanceMap1[b.Asset.ID] = b.Quantity
	}
	for _, b := range state.account.GetBalances() {
		balanceMap2[b.Asset.ID] = b.Quantity
	}

	for k, b1 := range balanceMap1 {
		b2 := balanceMap2[k]
		diff := math.Abs(b1-b2) / math.Abs(b1+b2)
		if diff > 0.01 {
			return fmt.Errorf("different margin amount: %f %f", state.account.GetMargin(nil), balanceList.Balances[0].Quantity)
		}
	}

	return nil
}
