package tests

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/gogo/protobuf/types"
	uuid "github.com/satori/go.uuid"
	"gitlab.com/alphaticks/alphac/exchanges"
	"gitlab.com/alphaticks/alphac/models"
	"gitlab.com/alphaticks/alphac/models/messages"
	"gitlab.com/alphaticks/xchanger/constants"
	"gitlab.com/alphaticks/xchanger/exchanges/bitmex"
	xchangerModels "gitlab.com/alphaticks/xchanger/models"
	"os"
	"reflect"
	"testing"
	"time"
)

var instrument1 = &models.Instrument{
	SecurityID: &types.UInt64Value{Value: 5391998915988476130},
	Exchange:   &constants.BITMEX,
	Symbol:     &types.StringValue{Value: "XBTUSD"},
}
var instrument2 = &models.Instrument{
	SecurityID: &types.UInt64Value{Value: 11093839049553737303},
	Exchange:   &constants.BITMEX,
	Symbol:     &types.StringValue{Value: "ETHUSD"},
}

var testAccount = &models.Account{
	AccountID: "299210",
	Exchange:  &constants.BITMEX,
	Credentials: &xchangerModels.APICredentials{
		APIKey:    "k5k6Mmaq3xe88Ph3fgIk9Vrt",
		APISecret: "0laIjZaKOMkJPtKy2ldJ18m4Dxjp66Vdim0k1-q4TXASZFZo",
	},
}

var executor *actor.PID

func TestMain(m *testing.M) {
	executor, _ = actor.EmptyRootContext.SpawnNamed(actor.PropsFromProducer(exchanges.NewExecutorProducer([]*xchangerModels.Exchange{&constants.BITMEX}, []*models.Account{testAccount})), "executor")
	bitmex.EnableTestNet()
	code := m.Run()
	bitmex.DisableTestNet()
	actor.EmptyRootContext.PoisonFuture(executor)
	os.Exit(code)
}

func TestAccountListener_OnOrderStatusRequest(t *testing.T) {
	// Test with no account
	res, err := actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   nil,
	}, 10*time.Second).Result()

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
	if orderList.RejectionReason != messages.InvalidAccount {
		t.Fatalf("was expecting %s, got %s", messages.InvalidAccount.String(), orderList.RejectionReason.String())
	}
	if len(orderList.Orders) > 0 {
		t.Fatalf("was expecting no order")
	}

	// Test with account
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
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

	// Test with instrument and order status
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			OrderID:       nil,
			ClientOrderID: nil,
			Instrument:    instrument1,
			OrderStatus:   &messages.OrderStatusValue{Value: models.New},
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

	// Test with one order
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   testAccount,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    instrument1,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      1.,
			Price:         &types.DoubleValue{Value: 100.},
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

	// Test with instrument and order status
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			OrderID:       nil,
			ClientOrderID: nil,
			Instrument:    instrument1,
			OrderStatus:   &messages.OrderStatusValue{Value: models.New},
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
		t.Fatalf("was expecting 1 open order, got %d", len(orderList.Orders))
	}
	order := orderList.Orders[0]
	if order.OrderStatus != models.New {
		t.Fatalf("order status not new")
	}
	if int(order.LeavesQuantity) != 1 {
		t.Fatalf("was expecting leaves quantity of 1")
	}
	if int(order.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order.OrderType != models.Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order.TimeInForce != models.Session {
		t.Fatalf("was expecting session time in force")
	}
	if order.Side != models.Buy {
		t.Fatalf("was expecting buy side order")
	}

	// Now delete
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderMassCancelRequest{
		RequestID: 0,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			Instrument: instrument1,
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

	// Query order and check if got canceled
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			OrderID:    &types.StringValue{Value: order.OrderID},
			Instrument: instrument1,
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
		t.Fatalf("was expecting 1 open order, got %d", len(orderList.Orders))
	}
	order = orderList.Orders[0]
	if order.OrderStatus != models.Canceled {
		t.Fatalf("order status not Canceled")
	}
	if int(order.LeavesQuantity) != 0 {
		t.Fatalf("was expecting leaves quantity of 0")
	}
	if int(order.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
}

func TestAccountListener_OnNewOrderSingleRequest(t *testing.T) {
	// Test Invalid account
	res, err := actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   nil,
		Order:     nil,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if response.Success {
		t.Fatalf("was expecting unsucessful request")
	}
	if response.RejectionReason != messages.InvalidAccount {
		t.Fatalf("was expecting %s got %s", messages.InvalidAccount.String(), response.RejectionReason.String())
	}

	// Test no order
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   testAccount,
		Order:     nil,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if response.Success {
		t.Fatalf("was expecting unsucessful request")
	}
	if response.RejectionReason != messages.InvalidRequest {
		t.Fatalf("was expecting %s got %s", messages.InvalidRequest.String(), response.RejectionReason.String())
	}

	// Test with one order
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderSingleRequest{
		RequestID: 0,
		Account:   testAccount,
		Order: &messages.NewOrder{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    instrument1,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      1.,
			Price:         &types.DoubleValue{Value: 100.},
		},
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.NewOrderSingleResponse)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderSingleResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	// Delete orders
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderMassCancelRequest{
		RequestID: 0,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			Instrument: instrument1,
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

func TestAccountListener_OnNewOrderBulkRequest(t *testing.T) {
	// Test Invalid account
	res, err := actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderBulkRequest{
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
	if response.RejectionReason != messages.InvalidAccount {
		t.Fatalf("was expecting %s got %s", messages.InvalidAccount.String(), response.RejectionReason.String())
	}

	// Test no orders
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   testAccount,
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
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   testAccount,
		Orders: []*messages.NewOrder{{
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    instrument1,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      1.,
			Price:         &types.DoubleValue{Value: 100.},
		}, {
			ClientOrderID: uuid.NewV1().String(),
			Instrument:    instrument2,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      1.,
			Price:         &types.DoubleValue{Value: 100.},
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
	if response.RejectionReason != messages.DifferentSymbols {
		t.Fatalf("was expecting %s, got %s", messages.DifferentSymbols.String(), response.RejectionReason.String())
	}

	order1ClID := uuid.NewV1().String()
	order2ClID := uuid.NewV1().String()
	// Test with two orders same symbol diff price
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.NewOrderBulkRequest{
		RequestID: 0,
		Account:   testAccount,
		Orders: []*messages.NewOrder{{
			ClientOrderID: order1ClID,
			Instrument:    instrument1,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      2,
			Price:         &types.DoubleValue{Value: 100.},
		}, {
			ClientOrderID: order2ClID,
			Instrument:    instrument1,
			OrderType:     models.Limit,
			OrderSide:     models.Buy,
			TimeInForce:   models.Session,
			Quantity:      1,
			Price:         &types.DoubleValue{Value: 110.},
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

	// Query order
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			ClientOrderID: &types.StringValue{Value: order1ClID},
			Instrument:    instrument1,
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
	if order1.OrderStatus != models.New {
		t.Fatalf("order status not new")
	}
	if int(order1.LeavesQuantity) != 2 {
		t.Fatalf("was expecting leaves quantity of 2")
	}
	if int(order1.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order1.OrderType != models.Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order1.TimeInForce != models.Session {
		t.Fatalf("was expecting session time in force")
	}
	if order1.Side != models.Buy {
		t.Fatalf("was expecting buy side order")
	}

	// Query order
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Subscribe: false,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			ClientOrderID: &types.StringValue{Value: order2ClID},
			Instrument:    instrument1,
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
	if order2.OrderStatus != models.New {
		t.Fatalf("order status not new")
	}
	if int(order2.LeavesQuantity) != 1 {
		t.Fatalf("was expecting leaves quantity of 2")
	}
	if int(order2.CumQuantity) != 0 {
		t.Fatalf("was expecting cum quantity of 0")
	}
	if order2.OrderType != models.Limit {
		t.Fatalf("was expecting limit order type")
	}
	if order2.TimeInForce != models.Session {
		t.Fatalf("was expecting session time in force")
	}
	if order2.Side != models.Buy {
		t.Fatalf("was expecting buy side order")
	}

	// Delete orders
	res, err = actor.EmptyRootContext.RequestFuture(executor, &messages.OrderMassCancelRequest{
		RequestID: 0,
		Account:   testAccount,
		Filter: &messages.OrderFilter{
			Instrument: instrument1,
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
