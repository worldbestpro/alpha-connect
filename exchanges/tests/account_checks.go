package tests

import (
	"fmt"
	"github.com/AsynkronIT/protoactor-go/actor"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"math"
	"reflect"
	"testing"
	"time"
)

func checkBalances(t *testing.T, as *actor.ActorSystem, executor *actor.PID, account *models.Account) {
	// Now check balance
	exchangeExecutor := as.NewLocalPID(fmt.Sprintf("executor/%s_executor", account.Exchange.Name))
	res, err := as.Root.RequestFuture(executor, &messages.BalancesRequest{
		RequestID: 0,
		Account:   account,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	balanceResponse, ok := res.(*messages.BalanceList)
	if !ok {
		t.Fatalf("was expecting *messages.BalanceList, got %s", reflect.TypeOf(res).String())
	}
	if !balanceResponse.Success {
		t.Fatalf("was expecting sucessful request: %s", balanceResponse.RejectionReason.String())
	}

	bal1 := make(map[string]float64)
	for _, b := range balanceResponse.Balances {
		bal1[b.Asset.Name] = b.Quantity
	}
	fmt.Println("ACCOUNT BALANCE", bal1)

	res, err = as.Root.RequestFuture(exchangeExecutor, &messages.BalancesRequest{
		RequestID: 0,
		Account:   account,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	balanceResponse, ok = res.(*messages.BalanceList)
	if !ok {
		t.Fatalf("was expecting *messages.BalanceList, got %s", reflect.TypeOf(res).String())
	}
	if !balanceResponse.Success {
		t.Fatalf("was expecting sucessful request: %s", balanceResponse.RejectionReason.String())
	}

	bal2 := make(map[string]float64)
	for _, b := range balanceResponse.Balances {
		bal2[b.Asset.Name] = b.Quantity
	}
	fmt.Println("EXECUTOR BALANCE", bal2)

	for k, b1 := range bal1 {
		if b2, ok := bal2[k]; ok {
			if math.Abs(b1-b2) > 0.00001 {
				t.Fatalf("different balance for %s %f:%f", k, b1, b2)
			}
		} else {
			t.Fatalf("different balance for %s %f:%f", k, b1, 0.)
		}
	}
}

func checkPositions(t *testing.T, as *actor.ActorSystem, executor *actor.PID, account *models.Account, instrument *models.Instrument) {
	// Request the same from binance directly
	exchangeExecutor := as.NewLocalPID(fmt.Sprintf("executor/%s_executor", account.Exchange.Name))

	res, err := as.Root.RequestFuture(executor, &messages.PositionsRequest{
		RequestID:  0,
		Account:    account,
		Instrument: instrument,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.PositionList)
	if !ok {
		t.Fatalf("was expecting *messages.PositionList, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	pos1 := response.Positions

	res, err = as.Root.RequestFuture(exchangeExecutor, &messages.PositionsRequest{
		RequestID:  0,
		Account:    account,
		Instrument: instrument,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.PositionList)
	if !ok {
		t.Fatalf("was expecting *messages.NewOrderBulkResponse, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}
	pos2 := response.Positions

	if len(pos1) != len(pos2) {
		t.Fatalf("got different number of positions: %d %d", len(pos1), len(pos2))
	}

	for i := range pos1 {
		p1 := pos1[i]
		p2 := pos2[i]
		// Compare the two
		fmt.Println(p1.Cost, p2.Cost)
		if math.Abs(p1.Cost-p2.Cost) > 0.000001 {
			t.Fatalf("different cost %f:%f", p1.Cost, p2.Cost)
		}
		if math.Abs(p1.Quantity-p2.Quantity) > 0.00001 {
			t.Fatalf("different quantity %f:%f", p1.Quantity, p2.Quantity)
		}
	}
}

func checkOrders(t *testing.T, as *actor.ActorSystem, executor *actor.PID, account *models.Account) {
	// Request the same from binance directly
	exchangeExecutor := as.NewLocalPID(fmt.Sprintf("executor/%s_executor", account.Exchange.Name))

	res, err := as.Root.RequestFuture(executor, &messages.OrderStatusRequest{
		RequestID: 0,
		Account:   account,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok := res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}

	orders1 := response.Orders

	res, err = as.Root.RequestFuture(exchangeExecutor, &messages.OrderStatusRequest{
		RequestID: 0,
		Account:   account,
	}, 10*time.Second).Result()

	if err != nil {
		t.Fatal(err)
	}
	response, ok = res.(*messages.OrderList)
	if !ok {
		t.Fatalf("was expecting *messages.OrderList, got %s", reflect.TypeOf(res).String())
	}
	if !response.Success {
		t.Fatalf("was expecting sucessful request: %s", response.RejectionReason.String())
	}
	orders2 := response.Orders

	fmt.Println(orders1)
	fmt.Println(orders2)
}