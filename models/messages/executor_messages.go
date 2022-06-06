package messages

import (
	"github.com/asynkron/protoactor-go/actor"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"gitlab.com/alphaticks/xchanger/models"
	"time"
)

type BlockNumberRequest struct {
	RequestID uint64
	Chain     *models.Chain
}

type BlockNumberResponse struct {
	RequestID       uint64
	ResponseID      uint64
	Success         bool
	RejectionReason RejectionReason
	BlockNumber     uint64
}

type EVMContractCallRequest struct {
	RequestID   uint64
	Chain       *models.Chain
	Msg         ethereum.CallMsg
	BlockNumber uint64
}

type EVMContractCallResponse struct {
	RequestID       uint64
	ResponseID      uint64
	Out             []byte
	Success         bool
	RejectionReason RejectionReason
}

type EVMLogsQueryRequest struct {
	RequestID uint64
	Chain     *models.Chain
	Query     ethereum.FilterQuery
}

type EVMLogsQueryResponse struct {
	RequestID       uint64
	ResponseID      uint64
	Success         bool
	RejectionReason RejectionReason
	Logs            []types.Log
	Times           []uint64
}

type EVMLogsSubscribeRequest struct {
	RequestID  uint64
	Chain      *models.Chain
	Query      ethereum.FilterQuery
	Subscriber *actor.PID
}

type EVMLogsSubscribeResponse struct {
	RequestID       uint64
	ResponseID      uint64
	Success         bool
	RejectionReason RejectionReason
	SeqNum          uint64
}

type EVMLogsSubscribeRefresh struct {
	RequestID uint64
	SeqNum    uint64
	Update    *EVMLogs
}

type EVMLogs struct {
	BlockNumber uint64
	BlockTime   time.Time
	Logs        []types.Log
}