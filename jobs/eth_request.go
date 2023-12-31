package jobs

import (
	goContext "context"
	"math/big"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// An api query actor execute a query and fits the result back into a given types

// The query actor panic when the request doesn't succeed (timeout etc..)
// The query actor doesn't panic when the request was successful but the server
// returned an error (client or server error)

type PerformLogsQueryRequest struct {
	Query ethereum.FilterQuery
}

// We allow this query to fail without crashing
// because the failure is outside the system
// We are not responsible for the failure
type PerformLogsQueryResponse struct {
	Error error
	Logs  []types.Log
	Times []uint64
}

type ETHQuery struct {
	client *ethclient.Client
}

func NewETHQuery(client *ethclient.Client) actor.Actor {
	return &ETHQuery{client}
}

func (q *ETHQuery) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		err := q.Initialize(context)
		if err != nil {
			panic(err)
		}

	case *actor.Stopping:
		if err := q.Clean(context); err != nil {
			panic(err)
		}

	case *actor.Restarting:
		q.Clean(context)

	case *PerformLogsQueryRequest:
		//Set API credentials
		err := q.PerformLogsQueryRequest(context)
		if err != nil {
			panic(err)
		}
	}
}

func (q *ETHQuery) Initialize(context actor.Context) error {
	return nil
}

func (q *ETHQuery) Clean(context actor.Context) error {
	return nil
}

func (q *ETHQuery) PerformLogsQueryRequest(context actor.Context) error {
	msg := context.Message().(*PerformLogsQueryRequest)
	go func(sender *actor.PID) {
		queryResponse := PerformLogsQueryResponse{}
		logs, err := q.client.FilterLogs(goContext.Background(), msg.Query)
		if err != nil {
			queryResponse.Error = err
			context.Send(sender, &queryResponse)
			return
		} else {
			queryResponse.Logs = logs
		}
		var lastBlock uint64 = 0
		var lastTime uint64 = 0
		for _, l := range logs {
			if lastBlock != l.BlockNumber {
				block, err := q.client.HeaderByNumber(goContext.Background(), big.NewInt(int64(l.BlockNumber)))
				if err != nil {
					queryResponse.Error = err
					context.Send(sender, &queryResponse)
					return
				}
				lastTime = block.Time
				lastBlock = l.BlockNumber
			}
			queryResponse.Times = append(queryResponse.Times, lastTime)
		}
		context.Send(sender, &queryResponse)
	}(context.Sender())

	return nil
}
