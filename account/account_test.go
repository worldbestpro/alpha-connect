package account

import (
	"github.com/gogo/protobuf/types"
	"gitlab.com/alphaticks/alphac/enum"
	"gitlab.com/alphaticks/alphac/models"
	"gitlab.com/alphaticks/xchanger/constants"
	"math"
	"testing"
)

var BTCUSD_PERP_SEC = &models.Security{
	SecurityID:        0,
	SecurityType:      enum.SecurityType_CRYPTO_PERP,
	Exchange:          &constants.BITMEX,
	Symbol:            "XBTUSD",
	MinPriceIncrement: 0.05,
	RoundLot:          1,
	Underlying:        &constants.BITCOIN,
	QuoteCurrency:     &constants.DOLLAR,
	Enabled:           true,
	IsInverse:         true,
	MakerFee:          &types.DoubleValue{Value: -0.00025},
	TakerFee:          &types.DoubleValue{Value: 0.00075},
	Multiplier:        &types.DoubleValue{Value: -1.},
	MaturityDate:      nil,
}

var ETHUSD_PERP_SEC = &models.Security{
	SecurityID:        1,
	SecurityType:      enum.SecurityType_CRYPTO_PERP,
	Exchange:          &constants.BITMEX,
	Symbol:            "ETHUSD",
	MinPriceIncrement: 0.05,
	RoundLot:          1,
	Underlying:        &constants.ETHEREUM,
	QuoteCurrency:     &constants.DOLLAR,
	Enabled:           true,
	IsInverse:         false,
	MakerFee:          &types.DoubleValue{Value: -0.00025},
	TakerFee:          &types.DoubleValue{Value: 0.00075},
	Multiplier:        &types.DoubleValue{Value: 0.000001},
	MaturityDate:      nil,
}

var BTCUSD_SPOT_SEC = &models.Security{
	SecurityID:        2,
	SecurityType:      enum.SecurityType_CRYPTO_SPOT,
	Exchange:          &constants.BITSTAMP,
	Symbol:            "BTCUSD",
	MinPriceIncrement: 0.05,
	RoundLot:          0.0001,
	Underlying:        &constants.BITCOIN,
	QuoteCurrency:     &constants.DOLLAR,
	Enabled:           true,
	IsInverse:         false,
	MakerFee:          &types.DoubleValue{Value: 0.0025},
	TakerFee:          &types.DoubleValue{Value: 0.0025},
	MaturityDate:      nil,
}

var ETHUSD_SPOT_SEC = &models.Security{
	SecurityID:        3,
	SecurityType:      enum.SecurityType_CRYPTO_SPOT,
	Exchange:          &constants.BITSTAMP,
	Symbol:            "ETHUSD",
	MinPriceIncrement: 0.05,
	RoundLot:          0.0001,
	Underlying:        &constants.ETHEREUM,
	QuoteCurrency:     &constants.DOLLAR,
	Enabled:           true,
	IsInverse:         false,
	MakerFee:          &types.DoubleValue{Value: 0.0025},
	TakerFee:          &types.DoubleValue{Value: 0.0025},
	MaturityDate:      nil,
}

func TestAccount_ConfirmFill(t *testing.T) {
	accnt := NewAccount("a", []*models.Security{ETHUSD_PERP_SEC}, &constants.BITCOIN, 1./0.00000001)
	err := accnt.Sync(nil, nil, nil, 0.)
	if err != nil {
		t.Fatal(err)
	}

	// Add a buy order
	_, rej := accnt.NewOrder(&models.Order{
		OrderID:       "buy",
		ClientOrderID: "buy",
		Instrument: &models.Instrument{
			SecurityID: &types.UInt64Value{Value: 1},
			Exchange:   &constants.BITMEX,
			Symbol:     &types.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.PendingNew,
		OrderType:      models.Limit,
		Side:           models.Buy,
		TimeInForce:    models.Session,
		LeavesQuantity: 10.,
		CumQuantity:    0,
		Price:          &types.DoubleValue{Value: 10.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy", "buy")
	if err != nil {
		t.Fatal(err)
	}

	// Add a sell order
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "sell",
		ClientOrderID: "sell",
		Instrument: &models.Instrument{
			SecurityID: &types.UInt64Value{Value: 1},
			Exchange:   &constants.BITMEX,
			Symbol:     &types.StringValue{Value: "ETHUSD"},
		},
		OrderStatus:    models.PendingNew,
		OrderType:      models.Limit,
		Side:           models.Sell,
		TimeInForce:    models.Session,
		LeavesQuantity: 10.,
		CumQuantity:    0,
		Price:          &types.DoubleValue{Value: 10.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("sell", "sell")
	if err != nil {
		t.Fatal(err)
	}

	fee1 := math.Floor(0.00025*200*2*0.000001*accnt.marginPrecision) / accnt.marginPrecision
	fee2 := math.Floor(0.00025*210*2*0.000001*accnt.marginPrecision) / accnt.marginPrecision
	expectedMarginChange := ((210 - 200) * 2 * 0.000001) + fee1 + fee2

	_, err = accnt.ConfirmFill("sell", "k1", 210., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("sell", "k1", 210., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 2., false)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(accnt.GetMargin()-expectedMarginChange) > 0.000000001 {
		t.Fatalf("was expecting margin of %g, got %g", expectedMarginChange, accnt.GetMargin())
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 2., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("sell", "k1", 210., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("sell", "k1", 210., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	expectedMarginChange = expectedMarginChange + expectedMarginChange
	if math.Abs(accnt.GetMargin()-expectedMarginChange) > 0.000000001 {
		t.Fatalf("was expecting margin of %g, got %g", expectedMarginChange, accnt.GetMargin())
	}
}

func TestAccount_ConfirmFill_Inverse(t *testing.T) {
	accnt := NewAccount("a", []*models.Security{BTCUSD_PERP_SEC}, &constants.BITCOIN, 1./0.00000001)
	err := accnt.Sync(nil, nil, nil, 0.)
	if err != nil {
		t.Fatal(err)
	}

	// Add a buy order
	_, rej := accnt.NewOrder(&models.Order{
		OrderID:       "buy",
		ClientOrderID: "buy",
		Instrument: &models.Instrument{
			SecurityID: &types.UInt64Value{Value: 0},
			Exchange:   &constants.BITMEX,
			Symbol:     &types.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.PendingNew,
		OrderType:      models.Limit,
		Side:           models.Buy,
		TimeInForce:    models.Session,
		LeavesQuantity: 10.,
		CumQuantity:    0,
		Price:          &types.DoubleValue{Value: 10.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("buy", "buy")
	if err != nil {
		t.Fatal(err)
	}

	// Add a sell order
	_, rej = accnt.NewOrder(&models.Order{
		OrderID:       "sell",
		ClientOrderID: "sell",
		Instrument: &models.Instrument{
			SecurityID: &types.UInt64Value{Value: 0},
			Exchange:   &constants.BITMEX,
			Symbol:     &types.StringValue{Value: "XBTUSD"},
		},
		OrderStatus:    models.PendingNew,
		OrderType:      models.Limit,
		Side:           models.Sell,
		TimeInForce:    models.Session,
		LeavesQuantity: 10.,
		CumQuantity:    0,
		Price:          &types.DoubleValue{Value: 10.},
	})
	if rej != nil {
		t.Fatalf(rej.String())
	}
	_, err = accnt.ConfirmNewOrder("sell", "sell")
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("sell", "k1", 210., 2., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	fee1 := math.Floor(0.00025*(1./200.)*2.*accnt.marginPrecision) / accnt.marginPrecision
	fee2 := math.Floor(0.00025*(1./210.)*2.*accnt.marginPrecision) / accnt.marginPrecision

	cost1 := (math.Round(1./200.*accnt.marginPrecision) / accnt.marginPrecision) * 2.
	cost2 := (math.Round(1./210.*accnt.marginPrecision) / accnt.marginPrecision) * 2.
	expectedMarginChange := (cost1 - cost2) + fee1 + fee2
	if math.Abs(accnt.GetMargin()-expectedMarginChange) > 0.00000001 {
		t.Fatalf("was expecting margin of %g, got %g", expectedMarginChange, accnt.GetMargin())
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("buy", "k1", 200., 1., false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = accnt.ConfirmFill("sell", "k1", 210., 2., false)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(accnt.GetMargin()-2*expectedMarginChange) > 0.00000001 {
		t.Fatalf("was expecting margin of %g, got %g", 2*expectedMarginChange, accnt.GetMargin())
	}
}