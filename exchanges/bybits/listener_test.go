package bybits_test

import (
	"gitlab.com/alphaticks/alpha-connect/enum"
	"gitlab.com/alphaticks/alpha-connect/exchanges/tests"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/xchanger/constants"
	"testing"
)

func TestMarketData(t *testing.T) {
	tests.MarketData(t, tests.MDTest{
		SecurityID:        5680225275551952603,
		Symbol:            "BTCUSDT",
		SecurityType:      enum.SecurityType_CRYPTO_SPOT,
		Exchange:          constants.BYBITS,
		BaseCurrency:      constants.BITCOIN,
		QuoteCurrency:     constants.TETHER,
		MinPriceIncrement: 0.01,
		RoundLot:          1e-06,
		HasMaturityDate:   false,
		IsInverse:         false,
		Status:            models.Trading,
	})
}

func TestMarketData2(t *testing.T) {
	tests.MarketData(t, tests.MDTest{
		SecurityID:        7604099800167109686,
		Symbol:            "LTCUSDT",
		SecurityType:      enum.SecurityType_CRYPTO_SPOT,
		Exchange:          constants.BYBITS,
		BaseCurrency:      constants.LITECOIN,
		QuoteCurrency:     constants.TETHER,
		MinPriceIncrement: 0.01,
		RoundLot:          1e-05,
		HasMaturityDate:   false,
		IsInverse:         false,
		Status:            models.Trading,
	})
}
