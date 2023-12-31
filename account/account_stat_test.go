package account_test

import (
	"gitlab.com/alphaticks/alpha-connect/account"
	"gitlab.com/alphaticks/alpha-connect/modeling"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/tests"
	"gitlab.com/alphaticks/xchanger/constants"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"math"
	"testing"
)

var model modeling.MarketModel

func InitModel(t *testing.T) {
	if err := tests.LoadStatics(); err != nil {
		t.Fatalf("error loading statics: %v", err)
	}
	mdl := modeling.NewMapMarketModel()
	mdl.SetPriceModel(uint64(constants.BITCOIN.ID)<<32|uint64(constants.TETHER.ID), modeling.NewConstantPriceModel(100.))
	mdl.SetPriceModel(uint64(constants.BITCOIN.ID)<<32|uint64(constants.DOLLAR.ID), modeling.NewConstantPriceModel(100.))
	mdl.SetPriceModel(uint64(constants.ETHEREUM.ID)<<32|uint64(constants.DOLLAR.ID), modeling.NewConstantPriceModel(10.))
	mdl.SetPriceModel(uint64(constants.DOLLAR.ID)<<32|uint64(constants.DOLLAR.ID), modeling.NewConstantPriceModel(1.))
	mdl.SetPriceModel(uint64(constants.TETHER.ID)<<32|uint64(constants.DOLLAR.ID), modeling.NewConstantPriceModel(1.))

	mdl.SetPriceModel(BTCUSDT_PERP_SEC.SecurityID, modeling.NewConstantPriceModel(100.))
	mdl.SetPriceModel(BTCUSD_PERP_SEC.SecurityID, modeling.NewConstantPriceModel(100.))
	mdl.SetPriceModel(ETHUSD_PERP_SEC.SecurityID, modeling.NewConstantPriceModel(10.))

	mdl.SetBuyTradeModel(BTCUSDT_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(2))
	mdl.SetBuyTradeModel(BTCUSD_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetBuyTradeModel(ETHUSD_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetBuyTradeModel(BTCUSD_SPOT_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetBuyTradeModel(ETHUSD_SPOT_SEC.SecurityID, modeling.NewConstantTradeModel(20))

	mdl.SetSellTradeModel(BTCUSDT_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(2))
	mdl.SetSellTradeModel(BTCUSD_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetSellTradeModel(ETHUSD_PERP_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetSellTradeModel(BTCUSD_SPOT_SEC.SecurityID, modeling.NewConstantTradeModel(20))
	mdl.SetSellTradeModel(ETHUSD_SPOT_SEC.SecurityID, modeling.NewConstantTradeModel(20))

	model = mdl

}

func TestAccount_GetAvailableMargin(t *testing.T) {
	InitModel(t)
	account, err := account.NewAccount(bitmexAccount, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// TODO 0.1 margin
	if err = account.Sync([]*models.Security{BTCUSD_PERP_SEC, ETHUSD_PERP_SEC}, nil, nil, []*models.Balance{{Asset: constants.BITCOIN, Quantity: 0.1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	expectedAv := 0.1
	avMargin, _ := account.GetNetMargin(model)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej := account.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: ETHUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: 10,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: 90.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = account.ConfirmNewOrder("buy1", "buy1", nil)
	if err != nil {
		t.Fatal(err)
	}
	account.ConfirmFill("buy1", "", 9., 10, false)
	// Balance + maker rebate + entry cost + PnL
	mul := ETHUSD_PERP_SEC.Multiplier.Value
	expectedAv = 0.1 + (0.00025 * 10 * 9 * mul) - (10 * 9 * mul) + (10.-9.)*mul*10.
	avMargin, _ = account.GetNetMargin(model)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}
}

/*
func TestAccount_GetAvailableMargin_Inverse(t *testing.T) {
	InitModel(t)
	account, err := account.NewAccount(bitmexAccount)
	if err != nil {
		t.Fatal(err)
	}
	// TODO 0.1 margin
	if err := account.Sync([]*models.Security{BTCUSD_PERP_SEC, ETHUSD_PERP_SEC}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	expectedAv := 0.1
	avMargin := account.GetMargin(model)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej := account.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: 10,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: 90.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = account.ConfirmNewOrder("buy1", "buy1")
	if err != nil {
		t.Fatal(err)
	}
	account.ConfirmFill("buy1", "", 90., 10, false)
	// Balance + maker rebate + entry cost + PnL
	expectedAv = 0.1 + (0.00025 * 10 * (1. / 90.)) - (10 * (1 / 90.)) + ((1./90.)-(1./100.))*10
	avMargin = account.GetAvailableMargin(model, 1.)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}
}

func TestAccount_PnL_Inverse(t *testing.T) {
	InitModel(t)
	account, err := account.NewAccount(bitmexAccount)
	if err != nil {
		t.Fatal(err)
	}
	// TODO 0.1 margin
	if err := account.Sync([]*models.Security{BTCUSD_PERP_SEC, ETHUSD_PERP_SEC}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	expectedAv := 0.1
	avMargin := account.GetAvailableMargin(model, 1.)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej := account.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: 100,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: 90.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = account.ConfirmNewOrder("buy1", "buy1")
	if err != nil {
		t.Fatal(err)
	}

	_, rej = account.NewOrder(&models.Order{
		OrderID:       "sell1",
		ClientOrderID: "sell1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Sell,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: 100,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: 110.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = account.ConfirmNewOrder("sell1", "sell1")
	if err != nil {
		t.Fatal(err)
	}

	account.ConfirmFill("buy1", "", 90., 10, false)
	// Balance + maker rebate + entry cost + PnL
	expectedAv = 0.1 + (0.00025 * 10 * (1. / 90.)) - (10 * (1 / 90.)) + ((1./90.)-(1./100.))*10
	avMargin = account.GetAvailableMargin(model, 1.)
	if math.Abs(avMargin-expectedAv) > 0.0000001 {
		t.Fatalf("was expecting %g, got %g", expectedAv, avMargin)
	}

	account.ConfirmFill("sell1", "", 110., 20, false)

	account.ConfirmFill("buy1", "", 90., 10, false)
	account.ConfirmFill("sell1", "", 110., 10, false)
	account.ConfirmFill("buy1", "", 90., 10, false)

}

func TestPortfolio_Spot_ELR(t *testing.T) {
	InitModel(t)
	accnt, err := account.NewAccount(bitstampAccount)
	if err != nil {
		t.Fatal(err)
	}
	dollarBalance := &models.Balance{
		Account:  "1",
		Asset:    constants.DOLLAR,
		Quantity: 100,
	}
	ethereumBalance := &models.Balance{
		Account:  "1",
		Asset:    constants.ETHEREUM,
		Quantity: 10,
	}
	if err := accnt.Sync([]*models.Security{BTCUSD_SPOT_SEC, ETHUSD_SPOT_SEC}, nil, nil, []*models.Balance{dollarBalance, ethereumBalance}, nil, nil); err != nil {
		t.Fatal(err)
	}

	p := account.NewPortfolio(1000)
	p.AddAccount(accnt)

	expectedBaseChange := 2 - (2 * 0.0025)
	expectedQuoteChange := 2 * 50.
	expectedValueChange := expectedBaseChange*100. - expectedQuoteChange
	expectedElr := math.Log((p.Value(model) + expectedValueChange) / p.Value(model))

	// The trade needs to be profitable or the portfolio will return us a nil order and an ELR of 0
	elr, o := p.GetELROnLimitBid("1", BTCUSD_SPOT_SEC.SecurityID, model, 10, []float64{50}, []float64{1}, 100.)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	o.Quantity = math.Round(o.Quantity/BTCUSD_SPOT_SEC.RoundLot.Value) * BTCUSD_SPOT_SEC.RoundLot.Value
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej := accnt.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_SPOT_SEC.SecurityID},
			Exchange:   constants.BITSTAMP,
			Symbol:     &wrapperspb.StringValue{Value: "BTCUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy1", "buy1")
	if err != nil {
		t.Fatal(err)
	}

	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	accnt.CancelOrder("buy1")
	if _, err := accnt.ConfirmCancelOrder("buy1"); err != nil {
		t.Fatal(err)
	}

	expectedElr = 0.
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedBaseChange = 10
	expectedQuoteChange = 10 * 20. * (1 - 0.0025)
	expectedValueChange = expectedQuoteChange - expectedBaseChange*10.
	expectedElr = math.Log((p.Value(model) + expectedValueChange) / p.Value(model))

	elr, o = p.GetELROnLimitAsk("1", ETHUSD_SPOT_SEC.SecurityID, model, 10, []float64{20}, []float64{1}, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	o.Quantity = math.Round(o.Quantity/ETHUSD_SPOT_SEC.RoundLot.Value) * ETHUSD_SPOT_SEC.RoundLot.Value
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "buy2",
		ClientOrderID: "buy2",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: ETHUSD_SPOT_SEC.SecurityID},
			Exchange:   constants.BITSTAMP,
			Symbol:     &wrapperspb.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Sell,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy2", "buy2")
	if err != nil {
		t.Fatal(err)
	}

	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	accnt.CancelOrder("buy2")
	if _, err := accnt.ConfirmCancelOrder("buy2"); err != nil {
		t.Fatal(err)
	}

	expectedElr = 0.
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
}

func TestPortfolio_Margin_ELR(t *testing.T) {
	InitModel(t)
	accnt, err := account.NewAccount(bitmexAccount)
	if err != nil {
		t.Fatal(err)
	}
	if err := accnt.Sync([]*models.Security{BTCUSD_PERP_SEC, ETHUSD_PERP_SEC}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	p := account.NewPortfolio(1000)
	p.AddAccount(accnt)

	expectedMarginChange := ((1./90 - 1./100) * 9) - (0.00075 * (1. / 90) * 9)
	expectedElr := math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, _ := p.GetELROnMarketBuy("1", BTCUSD_PERP_SEC.SecurityID, model, 10, 90, 9, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedMarginChange = ((1./90 - 1./100) * 9) + (0.00025 * (1. / 90) * 9)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))

	elr, o := p.GetELROnLimitBid("1", BTCUSD_PERP_SEC.SecurityID, model, 10, []float64{90}, []float64{1}, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej := accnt.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy1", "buy1")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateBidOrderQueue(BTCUSD_PERP_SEC.SecurityID, "buy1", 1)
	// Try with same time to test value cache consistency
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// Try with different time
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelBid("1", BTCUSD_PERP_SEC.SecurityID, "buy1", model, 11)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	accnt.CancelOrder("buy1")
	_, err = accnt.ConfirmCancelOrder("buy1")
	if err != nil {
		t.Fatal(err)
	}

	expectedElr = 0.
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// ETHUSD

	expectedMarginChange = ((10 - 9) * 0.000001 * 19) + (-0.00075 * 9. * 19 * 0.000001)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, _ = p.GetELROnMarketBuy("1", ETHUSD_PERP_SEC.SecurityID, model, 10, 9, 19, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedMarginChange = ((10 - 9) * 0.000001 * 19) + (0.00025 * 9. * 19 * 0.000001)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))

	elr, o = p.GetELROnLimitBid("1", ETHUSD_PERP_SEC.SecurityID, model, 10, []float64{9}, []float64{1}, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	o.Quantity = math.Round(o.Quantity)
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "buy2",
		ClientOrderID: "buy2",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: ETHUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy2", "buy2")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateBidOrderQueue(ETHUSD_PERP_SEC.SecurityID, "buy2", 1)

	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelBid("1", ETHUSD_PERP_SEC.SecurityID, "buy2", model, 11)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}
	accnt.CancelOrder("buy2")
	if _, err := accnt.ConfirmCancelOrder("buy2"); err != nil {
		t.Fatal(err)
	}

	expectedElr = 0.

	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// BTC USD short
	// Short 11

	expectedMarginChange = ((1./100 - 1./110) * 1. * 11.) + (-0.00075 * (1. / 110) * 11.)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, _ = p.GetELROnMarketSell("1", BTCUSD_PERP_SEC.SecurityID, model, 10, 110, 11, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedMarginChange = ((1./100 - 1./110) * 1. * 11.) + (0.00025 * (1. / 110) * 11.)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, o = p.GetELROnLimitAsk("1", BTCUSD_PERP_SEC.SecurityID, model, 10, []float64{110}, []float64{1}, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	o.Quantity = math.Round(o.Quantity)

	// Add a sell order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "sell1",
		ClientOrderID: "sell1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Sell,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("sell1", "sell1")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateAskOrderQueue(BTCUSD_PERP_SEC.SecurityID, "sell1", 1)

	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelAsk("1", BTCUSD_PERP_SEC.SecurityID, "sell1", model, 11)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}
	elr = p.GetELROnCancelAsk("1", BTCUSD_PERP_SEC.SecurityID, "sell1", model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	accnt.CancelOrder("sell1")
	if _, err := accnt.ConfirmCancelOrder("sell1"); err != nil {
		t.Fatal(err)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	// ETHUSD Short
	expectedMarginChange = ((11 - 10) * 0.000001 * 19) + (-0.00075 * 11. * 19 * 0.000001)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, _ = p.GetELROnMarketSell("1", ETHUSD_PERP_SEC.SecurityID, model, 11, 11, 19, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedMarginChange = ((11 - 10) * 0.000001 * 19) + (0.00025 * 11. * 19 * 0.000001)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange * 100)) / p.Value(model))
	elr, o = p.GetELROnLimitAsk("1", ETHUSD_PERP_SEC.SecurityID, model, 11, []float64{11}, []float64{1}, 0.1)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	o.Quantity = math.Round(o.Quantity)

	// Add a sell order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "sell2",
		ClientOrderID: "sell2",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: ETHUSD_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Sell,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("sell2", "sell2")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateAskOrderQueue(ETHUSD_PERP_SEC.SecurityID, "sell2", 1)

	elr = p.ExpectedLogReturn(model, 10)

	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelAsk("1", ETHUSD_PERP_SEC.SecurityID, "sell2", model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	accnt.CancelOrder("sell2")
	if _, err := accnt.ConfirmCancelOrder("sell2"); err != nil {
		t.Fatal(err)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}
}

/*
// Test case where you have multiple go routines accessing the portfolio together
// One go routine updates contracts, the other
func TestBitmexPortfolio_Parallel(t *testing.T) {
	gbm := NewGBMPriceModel(100., 100)
	priceModels := make(map[uint64]PriceModel)
	priceModels[bitmexInstruments[0].ID()] = gbm

	buyTradeModels := make(map[uint64]BuyTradeModel)
	buyTradeModels[bitmexInstruments[0].ID()] = NewConstantTradeModel(10)

	sellTradeModels := make(map[uint64]SellTradeModel)
	sellTradeModels[bitmexInstruments[0].ID()] = NewConstantTradeModel(10)

	ep, err := NewBitmexPortfolio(bitmexInstruments[:1], gbm, priceModels, buyTradeModels, sellTradeModels, 1000)
	if err != nil {
		t.Fatal(err)
	}
	p := NewPortfolio(map[uint64]ExchangePortfolio{bitmex: ep}, 1000)

	ep.SetWallet(1.)

	fmt.Println(p.ExpectedLogReturn(800))
	// One go routine place orders, and update wallet, etc

	// Others eval the price
}



func TestPortfolio_Fbinance_Margin_ELR(t *testing.T) {
	InitModel(t)
	accnt, err := account.NewAccount(fbinanceAccount)
	if err != nil {
		t.Fatal(err)
	}
	if err := accnt.Sync([]*models.Security{BTCUSDT_PERP_SEC}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	p := account.NewPortfolio(1000)
	p.AddAccount(accnt)

	expectedMarginChange := ((100 - 90) * 0.1) - (0.0004 * 90 * 0.1)
	expectedElr := math.Log((p.Value(model) + (expectedMarginChange)) / p.Value(model))
	elr, _ := p.GetELROnMarketBuy("1", BTCUSDT_PERP_SEC.SecurityID, model, 10, 90, 0.1, 1000)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// Match on bid of 2, queue of 1, match of 1
	expectedMarginChange = ((100 - 90) * 1) - (0.0002 * 90 * 1)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange)) / p.Value(model))

	elr, o := p.GetELROnLimitBid("1", BTCUSDT_PERP_SEC.SecurityID, model, 10, []float64{90}, []float64{1}, 1000)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	// Add a buy order. Using o.quantity allows us to check if the returned order's quantity is correct too
	o.Quantity = 1.
	_, rej := accnt.NewOrder(&models.Order{
		OrderID:       "buy1",
		ClientOrderID: "buy1",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSDT_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Buy,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy1", "buy1")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateBidOrderQueue(BTCUSDT_PERP_SEC.SecurityID, "buy1", 1)
	// Try with same time to test value cache consistency
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// Try with different time
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelBid("1", BTCUSDT_PERP_SEC.SecurityID, "buy1", model, 11)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	accnt.CancelOrder("buy1")
	_, err = accnt.ConfirmCancelOrder("buy1")
	if err != nil {
		t.Fatal(err)
	}

	expectedElr = 0.
	elr = p.ExpectedLogReturn(model, 11)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	// SHORT

	// ETHUSD Short
	expectedMarginChange = ((110 - 100) * 1) - (0.0004 * 110 * 1)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange)) / p.Value(model))
	elr, _ = p.GetELROnMarketSell("1", BTCUSDT_PERP_SEC.SecurityID, model, 11, 110, 1, 1000)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	expectedMarginChange = ((110 - 100) * 1) - (0.0002 * 110 * 1)
	expectedElr = math.Log((p.Value(model) + (expectedMarginChange)) / p.Value(model))
	elr, o = p.GetELROnLimitAsk("1", BTCUSDT_PERP_SEC.SecurityID, model, 11, []float64{110}, []float64{1}, 1000)
	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	o.Quantity = math.Round(o.Quantity)

	// Add a sell order. Using o.quantity allows us to check if the returned order's quantity is correct too
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "sell2",
		ClientOrderID: "sell2",
		Instrument: &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: BTCUSDT_PERP_SEC.SecurityID},
			Exchange:   constants.BITMEX,
			Symbol:     &wrapperspb.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.OrderStatus_PendingNew,
		OrderType:      models.OrderType_Limit,
		Side:           models.Side_Sell,
		TimeInForce:    models.TimeInForce_Session,
		LeavesQuantity: o.Quantity,
		CumQuantity:    0,
		Price:          &wrapperspb.DoubleValue{Value: o.Price},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("sell2", "sell2")
	if err != nil {
		t.Fatal(err)
	}
	accnt.UpdateAskOrderQueue(BTCUSDT_PERP_SEC.SecurityID, "sell2", 1)

	elr = p.ExpectedLogReturn(model, 10)

	if math.Abs(elr-expectedElr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", expectedElr, elr)
	}

	elr = p.GetELROnCancelAsk("1", BTCUSDT_PERP_SEC.SecurityID, "sell2", model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}

	accnt.CancelOrder("sell2")
	if _, err := accnt.ConfirmCancelOrder("sell2"); err != nil {
		t.Fatal(err)
	}
	elr = p.ExpectedLogReturn(model, 10)
	if math.Abs(elr) > 0.000001 {
		t.Fatalf("was expecting %f got %f", 0., elr)
	}
}
*/
