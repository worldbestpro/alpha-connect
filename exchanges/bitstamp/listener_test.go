package bitstamp_test

import (
	"gitlab.com/alphaticks/alpha-connect/enum"
	"gitlab.com/alphaticks/alpha-connect/exchanges/tests"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/xchanger/constants"
	"testing"
)

func TestMarketData(t *testing.T) {
	tests.MarketData(t, tests.MDTest{
		SecurityID:        5279696656781449381,
		Symbol:            "btcusd",
		SecurityType:      enum.SecurityType_CRYPTO_SPOT,
		Exchange:          constants.BITSTAMP,
		BaseCurrency:      constants.BITCOIN,
		QuoteCurrency:     constants.DOLLAR,
		MinPriceIncrement: 0.01,
		RoundLot:          0.00000001,
		HasMaturityDate:   false,
		IsInverse:         false,
		Status:            models.Trading,
	})
}
