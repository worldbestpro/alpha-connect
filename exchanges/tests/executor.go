package tests

import (
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"reflect"
	"testing"
	"time"
)

type ExPubTest struct {
	Instrument                    *models.Instrument
	SecurityListRequest           bool
	HistoricalLiquidationsRequest bool
	MarketStatisticsRequest       bool
	MarketDataRequest             bool
}

func ExPub(t *testing.T, tc ExPubTest) {
	as, executor, cleaner := StartExecutor(t, tc.Instrument.Exchange, nil)
	defer cleaner()

	if tc.SecurityListRequest {
		t.Run("SecurityListRequest", func(t *testing.T) {
			res, err := as.Root.RequestFuture(executor, &messages.SecurityListRequest{
				RequestID: 0,
			}, 10*time.Second).Result()
			if err != nil {
				t.Fatal(err)
			}
			v, ok := res.(*messages.SecurityList)
			if !ok {
				t.Fatalf("was expecting *messages.SecurityList, got %s", reflect.TypeOf(res).String())
			}
			if !v.Success {
				t.Fatalf("was expecting success, go %s", v.RejectionReason.String())
			}
		})
	}

	if tc.MarketDataRequest {
		t.Run("MarketDataRequest", func(t *testing.T) {
			res, err := as.Root.RequestFuture(executor, &messages.MarketDataRequest{
				RequestID:   0,
				Instrument:  tc.Instrument,
				Aggregation: models.L2,
			}, 10*time.Second).Result()
			if err != nil {
				t.Fatal(err)
			}
			v, ok := res.(*messages.MarketDataResponse)
			if !ok {
				t.Fatalf("was expecting *messages.SecurityList, got %s", reflect.TypeOf(res).String())
			}
			if !v.Success {
				t.Fatalf("was expecting success, go %s", v.RejectionReason.String())
			}
		})
	}

	if tc.MarketStatisticsRequest {
		t.Run("MarketStatisticsRequest", func(t *testing.T) {
			res, err := as.Root.RequestFuture(executor, &messages.MarketStatisticsRequest{
				RequestID:  0,
				Instrument: tc.Instrument,
				Statistics: []models.StatType{models.OpenInterest},
			}, 10*time.Second).Result()
			if err != nil {
				t.Fatal(err)
			}
			v, ok := res.(*messages.MarketStatisticsResponse)
			if !ok {
				t.Fatalf("was expecting *messages.MarketStatisticsResponse, got %s", reflect.TypeOf(res).String())
			}
			if !v.Success {
				t.Fatalf("was expecting success, go %s", v.RejectionReason.String())
			}
		})
	}
}