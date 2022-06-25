package tests

import (
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	uuid "github.com/satori/go.uuid"
	chtypes "gitlab.com/alphaticks/alpha-connect/chains/types"
	extypes "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	prtypes "gitlab.com/alphaticks/alpha-connect/protocols/types"
	"gitlab.com/alphaticks/alpha-connect/tests"
	xchangerModels "gitlab.com/alphaticks/xchanger/models"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"reflect"
	"testing"
	"time"
)

type AccountTest struct {
	Account                 *models.Account
	Instrument              *models.Instrument
	SkipCheckBalance        bool
	OrderStatusRequest      bool
	ExpiredOrder            bool
	NewOrderBulkRequest     bool
	GetPositionsLimit       bool
	OrderReplaceRequest     bool
	OrderBulkReplaceRequest bool
	GetPositionsMarket      bool
	OrderMassCancelRequest  bool
}

type AccountTestCtx struct {
	as       *actor.ActorSystem
	executor *actor.PID
}

func clean(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	filter := &messages.OrderFilter{
		Instrument: tc.Instrument,
	}
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderMassCancelRequest{
		Account: tc.Account,
		Filter:  filter,
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.OrderMassCancelResponse)
	if !ok {
		t.Fatalf("expecting *messages.OrderMassCancelResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("unsuccessfull OrderMassCancelResponse: %s", response.RejectionReason.String())
	}
}

func AccntTest(t *testing.T, tc AccountTest) {
	tests.LoadStatics(t)
	as, executor, cleaner := tests.StartExecutor(t, &extypes.ExecutorConfig{
		Exchanges:     []*xchangerModels.Exchange{tc.Account.Exchange},
		StrictAccount: true,
	}, &prtypes.ExecutorConfig{}, &chtypes.ExecutorConfig{}, tc.Account)
	defer cleaner()

	ctx := AccountTestCtx{
		as:       as,
		executor: executor,
	}

	t.Run("AccountInformationRequest", func(t *testing.T) {
		res, err := as.Root.RequestFuture(executor, &messages.AccountInformationRequest{
			RequestID: 0,
			Account:   tc.Account,
		}, 10*time.Second).Result()
		if err != nil {
			t.Fatal(err)
		}
		v, ok := res.(*messages.AccountInformationResponse)
		if !ok {
			t.Fatalf("was expecting *messages.AccountInformationResponse, got %s", reflect.TypeOf(res).String())
		}
		if !v.Success {
			t.Fatalf("was expecting success, go %s", v.RejectionReason.String())
		}
	})

	// Get security def
	res, err := as.Root.RequestFuture(executor, &messages.SecurityDefinitionRequest{
		Instrument: tc.Instrument,
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	sd, ok := res.(*messages.SecurityDefinitionResponse)
	if !ok {
		t.Fatalf("was expecting balance SecurityDefinitionRequest, got %s", reflect.TypeOf(res).String())
	}
	if !sd.Success {
		t.Fatal(sd.RejectionReason.String())
	}

	sec := sd.Security

	// Get balances
	res, err = as.Root.RequestFuture(executor, &messages.BalancesRequest{
		Asset:   nil,
		Account: tc.Account,
	}, 25*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	bl, ok := res.(*messages.BalanceList)
	if !ok {
		t.Fatalf("was expecting balance list, got %s", reflect.TypeOf(res).String())
	}
	if !bl.Success {
		t.Fatal(bl.RejectionReason.String())
	}

	var available float64
	fmt.Println("BALANCES", bl.Balances, len(bl.Balances))
	for _, bal := range bl.Balances {
		fmt.Println(bal.Quantity)
		if bal.Asset.ID == sec.QuoteCurrency.ID {
			available = bal.Quantity
		}
	}
	if available == 0. {
		t.Fatal("quote balance of 0, cannot test")
	}

	// Get market data
	/*
		res, err = as.Root.RequestFuture(executor, &messages.MarketDataRequest{
			RequestID:   0,
			Instrument:  tc.Instrument,
			Aggregation: models.OrderBookAggregation_L2,
		}, 10*time.Second).Result()
		if err != nil {
			t.Fatal(err)
		}
		v, ok := res.(*messages.MarketDataResponse)
		if !ok {
			t.Fatalf("was expecting *messages.MarketDataResponse, got %s", reflect.TypeOf(res).String())
		}
		if !v.Success {
			t.Fatalf("was expecting success, go %s", v.RejectionReason.String())
		}

	*/

	if tc.OrderStatusRequest {
		OrderStatusRequest(t, ctx, tc, messages.ResponseType_Result)
		OrderStatusRequest(t, ctx, tc, messages.ResponseType_Ack)
	}
	if tc.ExpiredOrder {
		ExpiredOrder(t, ctx, tc, messages.ResponseType_Result)
		ExpiredOrder(t, ctx, tc, messages.ResponseType_Ack)
	}
	if tc.GetPositionsLimit {
		GetPositionsLimitShort(t, ctx, tc)
	}
	if tc.GetPositionsMarket {
		GetPositionsMarket(t, ctx, tc)
	}
	if tc.NewOrderBulkRequest {
		NewOrderBulkRequest(t, ctx, tc)
	}
	if tc.OrderReplaceRequest {
		OrderReplaceRequest(t, ctx, tc)
	}
	if tc.OrderBulkReplaceRequest {
		OrderBulkReplaceRequest(t, ctx, tc)
	}
	if tc.OrderMassCancelRequest {
		OrderMassCancelRequest(t, ctx, tc)
	}
	clean(t, ctx, tc)
}

func OrderStatusRequest(t *testing.T, ctx AccountTestCtx, tc AccountTest, respType messages.ResponseType) {
	// Test with no account
	// TODO Finish testing
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   nil,
	}, 30*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	orderList, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if orderList.Success {
		t.Fatalf("wasn't expecting success")
	}
	if orderList.RejectionReason != messages.RejectionReason_InvalidAccount {
		t.Fatalf("was expecting %s, got %s", messages.RejectionReason_InvalidAccount.String(), orderList.RejectionReason.String())
	}
	if len(orderList.Orders) > 0 {
		t.Fatalf("was expecting no order")
	}

	// Test with account
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
	}, 30*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}

	// Test with instrument and order status
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			OrderID:       nil,
			ClientOrderID: nil,
			Instrument:    tc.Instrument,
			OrderStatus:   &messages.OrderStatusValue{Value: models.OrderStatus_New},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) > 0 {
		t.Fatalf("was expecting no open order, got %d", len(orderList.Orders))
	}

	orderID := fmt.Sprintf("%d", time.Now().UnixNano())
	// Test with one order
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID:         orderID,
			Instrument:            tc.Instrument,
			OrderType:             models.OrderType_Limit,
			OrderSide:             models.Side_Buy,
			TimeInForce:           models.TimeInForce_GoodTillCancel,
			Quantity:              0.001,
			Price:                 &wrapperspb.DoubleValue{Value: 20000.},
			ExecutionInstructions: []models.ExecutionInstruction{models.ExecutionInstruction_ParticipateDoNotInitiate},
		},
		ResponseType: respType,
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	time.Sleep(3 * time.Second)
	filter := &messages.OrderFilter{
		OrderStatus: &messages.OrderStatusValue{Value: models.OrderStatus_New},
		Instrument:  tc.Instrument,
	}
	checkOrders(t, ctx.as, ctx.executor, tc.Account, filter)

	// Test with instrument and order status
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter:    filter,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting 1 open order, got %d", len(orderList.Orders))
	}
	order := orderList.Orders[0]
	if order.OrderStatus != models.OrderStatus_New {
		t.Fatalf("order status not new")
	}
	if int(order.LeavesQuantity*1000) != 1 {
		t.Fatalf("was expecting leaves quantity of 0.0001, got %f", order.LeavesQuantity)
	}
	if int(order.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order.OrderType != models.OrderType_Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order.TimeInForce != models.TimeInForce_GoodTillCancel {
		t.Fatalf("was expecting GoodTillCancel time in force")
	}
	if order.Side != models.Side_Buy {
		t.Fatalf("was expecting buy side order")
	}

	fmt.Println("CANCELLING")
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderCancelRequest{
		RequestID:    0,
		Account:      tc.Account,
		Instrument:   tc.Instrument,
		OrderID:      &wrapperspb.StringValue{Value: order.OrderID},
		ResponseType: respType,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	mcResponse, ok := res.(*messages.OrderCancelResponse)
	if !ok {
		t.Fatalf("was expecting *messages.OrderCancelResponse, got %s", reflect.TypeOf(res).String())
	}
	if !mcResponse.Success {
		t.Fatalf("was expecting successful request: %s", response.RejectionReason.String())
	}

	time.Sleep(1 * time.Second)

	// Query order and check if got canceled
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			OrderID:    &wrapperspb.StringValue{Value: order.OrderID},
			Instrument: tc.Instrument,
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting 1 order, got %d", len(orderList.Orders))
	}
	order = orderList.Orders[0]
	if order.OrderStatus != models.OrderStatus_Canceled {
		t.Fatalf("order status not Canceled, but %s", order.OrderStatus)
	}
	if int(order.LeavesQuantity) != 0 {
		t.Fatalf("was expecting leaves quantity of 0")
	}
	if int(order.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}

	checkOrders(t, ctx.as, ctx.executor, tc.Account, filter)
}

func ExpiredOrder(t *testing.T, ctx AccountTestCtx, tc AccountTest, respType messages.ResponseType) {
	orderID := fmt.Sprintf("%d", time.Now().UnixNano())
	// Test with one order
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID:         orderID,
			Instrument:            tc.Instrument,
			OrderType:             models.OrderType_Limit,
			OrderSide:             models.Side_Buy,
			TimeInForce:           models.TimeInForce_GoodTillCancel,
			Quantity:              0.001,
			Price:                 &wrapperspb.DoubleValue{Value: 30000.},
			ExecutionInstructions: []models.ExecutionInstruction{models.ExecutionInstruction_ParticipateDoNotInitiate},
		},
		ResponseType: respType,
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	time.Sleep(3 * time.Second)
	filter := &messages.OrderFilter{
		OrderStatus: &messages.OrderStatusValue{Value: models.OrderStatus_New},
		Instrument:  tc.Instrument,
	}
	checkOrders(t, ctx.as, ctx.executor, tc.Account, filter)

	// Test with instrument and order status
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter:    filter,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 0 {
		t.Fatalf("was expecting 0 open order, got %d", len(orderList.Orders))
	}
}

func NewOrderBulkRequest(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	// Test Invalid account
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   nil,
		Orders:    nil,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderBulkResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if response.Success {
		t.Fatalf("was expecting unsucessful request")
	}
	if response.RejectionReason != messages.RejectionReason_InvalidAccount {
		t.Fatalf("was expecting %s got %s", messages.RejectionReason_InvalidAccount.String(), response.RejectionReason.String())
	}

	// Test no orders
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   tc.Account,
		Orders:    nil,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.NewOrderBulkResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting successful request: %s", response.RejectionReason.String())
	}

	// Test with two orders diff symbols
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   tc.Account,
		Orders: []*messages.NewOrder{{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      1.,
			Price:         &wrapperspb.DoubleValue{Value: 30000.},
		}, {
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      1.,
			Price:         &wrapperspb.DoubleValue{Value: 30000.},
		}},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.NewOrderBulkResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if response.Success {
		t.Fatalf("was expecting unsuccessful")
	}
	if response.RejectionReason != messages.RejectionReason_DifferentSymbols {
		t.Fatalf("was expecting %s, got %s", messages.RejectionReason_DifferentSymbols.String(), response.RejectionReason.String())
	}

	order1ClID := uuid.NewV1().String()
	order2ClID := uuid.NewV1().String()
	// Test with two orders same symbol diff price
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   tc.Account,
		Orders: []*messages.NewOrder{{
			ClientOrderID: order1ClID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      2,
			Price:         &wrapperspb.DoubleValue{Value: 30000.},
		}, {
			ClientOrderID: order2ClID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      1,
			Price:         &wrapperspb.DoubleValue{Value: 30010.},
		}},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.NewOrderBulkResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting successful request: %s", response.RejectionReason.String())
	}
	time.Sleep(1 * time.Second)

	// Query order
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			ClientOrderID: &wrapperspb.StringValue{Value: order1ClID},
			Instrument:    tc.Instrument,
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting 1 order, got %d", len(orderList.Orders))
	}
	order1 := orderList.Orders[0]
	if order1.OrderStatus != models.OrderStatus_New {
		t.Fatalf("order status not new %s", order1.OrderStatus.String())
	}
	if int(order1.LeavesQuantity) != 2 {
		t.Fatalf("was expecting leaves quantity of 2")
	}
	if int(order1.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order1.OrderType != models.OrderType_Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order1.TimeInForce != models.TimeInForce_Session {
		t.Fatalf("was expecting session time in force")
	}
	if order1.Side != models.Side_Buy {
		t.Fatalf("was expecting buy side order")
	}

	// Query order
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			ClientOrderID: &wrapperspb.StringValue{Value: order2ClID},
			Instrument:    tc.Instrument,
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting 1 order, got %d", len(orderList.Orders))
	}
	order2 := orderList.Orders[0]
	if order2.OrderStatus != models.OrderStatus_New {
		t.Fatalf("order status not new")
	}
	if int(order2.LeavesQuantity) != 1 {
		t.Fatalf("was expecting leaves quantity of 2")
	}
	if int(order2.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order2.OrderType != models.OrderType_Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order2.TimeInForce != models.TimeInForce_Session {
		t.Fatalf("was expecting session time in force")
	}
	if order2.Side != models.Side_Buy {
		t.Fatalf("was expecting buy side order")
	}

	// Delete orders
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderMassCancelRequest{
		RequestID: 0,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			Instrument: tc.Instrument,
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	mcResponse, ok := res.(*messages.OrderMassCancelResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if !mcResponse.Success {
		t.Fatalf("was expecting successful request: %s", response.RejectionReason.String())
	}
}

func GetPositionsLimit(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	// Market buy
	orderID := fmt.Sprintf("%d", time.Now().UnixNano())
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: orderID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			Price:         &wrapperspb.DoubleValue{Value: 80000},
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	newOrderResponse := res.(*messages.NewOrderSingleResponse)
	if !newOrderResponse.Success {
		t.Fatalf("error creating new order: %s", newOrderResponse.RejectionReason.String())
	}

	time.Sleep(3 * time.Second)

	checkBalances(t, ctx.as, ctx.executor, tc.Account)
	checkPositions(t, ctx.as, ctx.executor, tc.Account, tc.Instrument)
	fmt.Println("CLOSING")
	// Close position
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID:         uuid.NewV1().String(),
			Instrument:            tc.Instrument,
			OrderType:             models.OrderType_Limit,
			OrderSide:             models.Side_Sell,
			Price:                 &wrapperspb.DoubleValue{Value: 32000},
			Quantity:              0.001,
			ExecutionInstructions: []models.ExecutionInstruction{models.ExecutionInstruction_ReduceOnly},
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	checkBalances(t, ctx.as, ctx.executor, tc.Account)
	checkPositions(t, ctx.as, ctx.executor, tc.Account, tc.Instrument)
}

func GetPositionsLimitShort(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	// Market sell
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_GoodTillCancel,
			Price:         &wrapperspb.DoubleValue{Value: 28000},
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}
	newOrderResponse := res.(*messages.NewOrderSingleResponse)
	if !newOrderResponse.Success {
		t.Fatalf("error creating new order: %s", newOrderResponse.RejectionReason.String())
	}

	time.Sleep(2 * time.Second)

	if !tc.SkipCheckBalance {
		checkBalances(t, ctx.as, ctx.executor, tc.Account)
	}
	checkPositions(t, ctx.as, ctx.executor, tc.Account, tc.Instrument)
	fmt.Println("CLOSING")

	// Market buy
	orderID := fmt.Sprintf("%d", time.Now().UnixNano())
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID:         orderID,
			Instrument:            tc.Instrument,
			OrderType:             models.OrderType_Limit,
			OrderSide:             models.Side_Buy,
			TimeInForce:           models.TimeInForce_GoodTillCancel,
			Price:                 &wrapperspb.DoubleValue{Value: 80000},
			Quantity:              0.001,
			ExecutionInstructions: []models.ExecutionInstruction{models.ExecutionInstruction_ReduceOnly},
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	checkBalances(t, ctx.as, ctx.executor, tc.Account)
	checkPositions(t, ctx.as, ctx.executor, tc.Account, tc.Instrument)
}

func OrderReplaceRequest(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	orderID := uuid.NewV1().String()
	// Post one order
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: orderID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
			Price:         &wrapperspb.DoubleValue{Value: 35000.},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	time.Sleep(5 * time.Second)

	// Test replace quantity
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderReplaceRequest{
		RequestID:  0,
		Account:    tc.Account,
		Instrument: tc.Instrument,
		Update: &messages.OrderUpdate{
			OrderID:           nil,
			OrigClientOrderID: &wrapperspb.StringValue{Value: orderID},
			Quantity:          &wrapperspb.DoubleValue{Value: 0.002},
			Price:             nil,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	replaceResponse, ok := res.(*messages.OrderReplaceResponse)
	if !ok {
		t.Fatalf("was expecting *messages.OrderReplaceResponse. got %s", reflect.TypeOf(res).String())
	}
	if !replaceResponse.Success {
		t.Fatalf("was expecting successful request: %s", replaceResponse.RejectionReason.String())
	}

	time.Sleep(1 * time.Second)
	// Fetch orders
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			ClientOrderID: &wrapperspb.StringValue{Value: orderID},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting one order, got %d", len(orderList.Orders))
	}
	if orderList.Orders[0].LeavesQuantity != 0.002 {
		t.Fatalf("was expecting quantity of 0.002")
	}
}

func OrderBulkReplaceRequest(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	order1ClID := uuid.NewV1().String()
	order2ClID := uuid.NewV1().String()
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   tc.Account,
		Orders: []*messages.NewOrder{{
			ClientOrderID: order1ClID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      1,
			Price:         &wrapperspb.DoubleValue{Value: 30000.},
		}, {
			ClientOrderID: order2ClID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Limit,
			OrderSide:     models.Side_Buy,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      1,
			Price:         &wrapperspb.DoubleValue{Value: 30010.},
		}},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderBulkResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting successful request: %s", response.RejectionReason.String())
	}
	time.Sleep(1 * time.Second)

	// Test replace quantity
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderBulkReplaceRequest{
		RequestID:  0,
		Account:    tc.Account,
		Instrument: tc.Instrument,
		Updates: []*messages.OrderUpdate{
			{
				OrderID:           nil,
				OrigClientOrderID: &wrapperspb.StringValue{Value: order1ClID},
				Quantity:          &wrapperspb.DoubleValue{Value: 2},
				Price:             nil,
			}, {
				OrderID:           nil,
				OrigClientOrderID: &wrapperspb.StringValue{Value: order2ClID},
				Quantity:          &wrapperspb.DoubleValue{Value: 2},
				Price:             nil,
			},
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	replaceResponse, ok := res.(*messages.OrderBulkReplaceResponse)
	if !ok {
		t.Fatalf("was expecting *messages.OrderBulkReplaceResponse. got %s", reflect.TypeOf(res).String())
	}
	if !replaceResponse.Success {
		t.Fatalf("was expecting successful request: %s", replaceResponse.RejectionReason.String())
	}

	time.Sleep(1 * time.Second)

	// Fetch orders
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			ClientOrderID: &wrapperspb.StringValue{Value: order1ClID},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting one order, ogt %d", len(orderList.Orders))
	}
	if orderList.Orders[0].LeavesQuantity != 2 {
		t.Fatalf("was expecting quantity of 2")
	}

	// Fetch orders
	res, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   tc.Account,
		Filter: &messages.OrderFilter{
			ClientOrderID: &wrapperspb.StringValue{Value: order2ClID},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}

	orderList, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !orderList.Success {
		t.Fatalf("was expecting success: %s", orderList.RejectionReason.String())
	}
	if len(orderList.Orders) != 1 {
		t.Fatalf("was expecting one order, ogt %d", len(orderList.Orders))
	}
	if orderList.Orders[0].LeavesQuantity != 2 {
		t.Fatalf("was expecting quantity of 2")
	}
}

func GetPositionsMarket(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	// Market buy 1 contract
	orderID := fmt.Sprintf("%d", time.Now().UnixNano())
	res, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: orderID,
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Buy,
			Quantity:      0.005,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	newOrderResponse := res.(*messages.NewOrderSingleResponse)
	if !newOrderResponse.Success {
		t.Fatalf("error creating new order: %s", newOrderResponse.RejectionReason.String())
	}

	time.Sleep(3 * time.Second)

	checkBalances(t, ctx.as, ctx.executor, tc.Account)

	fmt.Println("CHECK 1 GOOD")

	// Market sell 1 contract
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	checkBalances(t, ctx.as, ctx.executor, tc.Account)

	fmt.Println("CLOSING")
	// Close position
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	checkBalances(t, ctx.as, ctx.executor, tc.Account)

	// Close position
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	checkBalances(t, ctx.as, ctx.executor, tc.Account)

	// Close position
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	checkBalances(t, ctx.as, ctx.executor, tc.Account)

	// Close position
	_, err = ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
		Account: tc.Account,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    tc.Instrument,
			OrderType:     models.OrderType_Market,
			OrderSide:     models.Side_Sell,
			TimeInForce:   models.TimeInForce_Session,
			Quantity:      0.001,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	checkBalances(t, ctx.as, ctx.executor, tc.Account)
}

func OrderMassCancelRequest(t *testing.T, ctx AccountTestCtx, tc AccountTest) {
	//Place Orders
	var id []string
	for i := 0; i < 3; i++ {
		id = append(id, uuid.NewV1().String())
		resp, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.NewOrderSingleRequest{
			Account: tc.Account,
			Order: &messages.NewOrder{
				ClientOrderID: id[i],
				Instrument:    tc.Instrument,
				OrderType:     models.OrderType_Limit,
				OrderSide:     models.Side_Buy,
				TimeInForce:   models.TimeInForce_GoodTillCancel,
				Quantity:      0.001 * float64(1+i),
				Price:         &wrapperspb.DoubleValue{Value: 30000.},
			},
		}, 10*time.Second).Result()
		if err != nil {
			t.Fatal(err)
		}

		singleOrder, ok := resp.(*messages.NewOrderSingleResponse)
		if !ok {
			t.Fatalf("expecting *messasges.NewOrderSingleRequest, got %s", reflect.TypeOf(resp).String())
		}
		if !singleOrder.Success {
			t.Fatal(singleOrder.RejectionReason.String())
		}
	}
	time.Sleep(30 * time.Second)
	filter := &messages.OrderFilter{
		Instrument:  tc.Instrument,
		OrderStatus: &messages.OrderStatusValue{Value: models.OrderStatus_New},
	}
	checkOrders(t, ctx.as, ctx.executor, tc.Account, filter)
	//Cancel all the orders
	resp, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderMassCancelRequest{
		Account: tc.Account,
		Filter: &messages.OrderFilter{
			Instrument: tc.Instrument,
		},
	}, 10*time.Second).Result()
	if err != nil {
		t.Fatal(err)
	}

	cancel, ok := resp.(*messages.OrderMassCancelResponse)
	if !ok {
		t.Fatalf("expecting *messasges.OrderMassCancelResponse, got %s", reflect.TypeOf(resp).String())
	}
	if !cancel.Success {
		t.Fatal(cancel.RejectionReason.String())
	}
	time.Sleep(20 * time.Second)
	filter = &messages.OrderFilter{
		Instrument:  tc.Instrument,
		OrderStatus: &messages.OrderStatusValue{Value: models.OrderStatus_New},
	}
	checkOrders(t, ctx.as, ctx.executor, tc.Account, filter)
	// Fetch all orders
	for i := 0; i < 3; i++ {
		resp, err := ctx.as.Root.RequestFuture(ctx.executor, &messages.OrderStatusRequest{
			Account: tc.Account,
			Filter: &messages.OrderFilter{
				ClientOrderID: &wrapperspb.StringValue{Value: id[i]},
			},
		}, 10*time.Second).Result()
		if err != nil {
			t.Fatal(err)
		}
		order, ok := resp.(*messages.OrderList)
		if !ok {
			t.Fatalf("expecting *messasges.OrderList, got %s", reflect.TypeOf(resp).String())
		}
		if !order.Success {
			t.Fatal(cancel.RejectionReason.String())
		}
		fmt.Println(order)
		if len(order.Orders) != 1 {
			t.Fatalf("was expecting 1 order got %d", len(order.Orders))
		}
		if order.Orders[0].OrderStatus.String() != "Canceled" {
			t.Fatalf("expecting Canceled, got %s", order.Orders[0].OrderStatus.String())
		}
	}
}
