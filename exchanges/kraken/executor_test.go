package kraken_test

import (
	"gitlab.com/alphaticks/alpha-connect/exchanges/tests"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/xchanger/constants"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"testing"
)

func TestExecutorPublic(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tests.ExPub(t, tests.ExPubTest{
		Instrument: &models.Instrument{
			Exchange: constants.KRAKEN,
			Symbol:   &wrapperspb.StringValue{Value: "XBT/USD"},
		},
		SecurityListRequest:           true,
		MarketDataRequest:             true,
		HistoricalLiquidationsRequest: false,
		OpenInterestRequest:           false,
	})
}
