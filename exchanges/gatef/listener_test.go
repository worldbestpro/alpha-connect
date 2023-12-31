package gatef_test

import (
	"gitlab.com/alphaticks/alpha-connect/enum"
	"gitlab.com/alphaticks/alpha-connect/exchanges/tests"
	"gitlab.com/alphaticks/alpha-connect/models"
	exTests "gitlab.com/alphaticks/alpha-connect/tests"
	"gitlab.com/alphaticks/xchanger/constants"
	"testing"
)

func TestMarketData(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	if err := exTests.LoadStatics(); err != nil {
		t.Fatal(err)
	}
	tests.MarketData(t, tests.MDTest{
		IgnoreSizeResidue: true,
		Symbol:            "BTC_USDT",
		SecurityType:      enum.SecurityType_CRYPTO_PERP,
		Exchange:          constants.GATEF,
		MinPriceIncrement: 0.1,
		RoundLot:          1,
		HasMaturityDate:   false,
		IsInverse:         false,
		Status:            models.InstrumentStatus_Trading,
	})
}
