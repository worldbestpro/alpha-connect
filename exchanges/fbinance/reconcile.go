package fbinance

import (
	goContext "context"
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/log"
	"github.com/melaurent/gotickfile/v2"
	"gitlab.com/alphaticks/alpha-connect/account"
	"gitlab.com/alphaticks/alpha-connect/config"
	extypes "gitlab.com/alphaticks/alpha-connect/exchanges/types"
	"gitlab.com/alphaticks/alpha-connect/models"
	"gitlab.com/alphaticks/alpha-connect/models/messages"
	"gitlab.com/alphaticks/alpha-connect/utils"
	registry "gitlab.com/alphaticks/alpha-public-registry-grpc"
	"gitlab.com/alphaticks/tickfunctors/market/portfolio"
	tickstore_types "gitlab.com/alphaticks/tickstore-types"
	"gitlab.com/alphaticks/xchanger/constants"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gorm.io/gorm"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

type reconcile struct{}

type AccountReconcile struct {
	extypes.BaseReconcile
	accountCfg       config.Account
	account          *models.Account
	dbAccount        *extypes.Account
	executor         *actor.PID
	logger           *log.Logger
	securities       map[uint64]*registry.Security
	symbToSecs       map[string]*registry.Security
	store            tickstore_types.TickstoreClient
	db               *gorm.DB
	registry         registry.StaticClient
	positions        map[uint64]*account.Position
	lastDepositTs    uint64
	lastWithdrawalTs uint64
	lastFundingTs    uint64
	lastTradeID      map[uint64]uint64
	reconcileTicker  *time.Ticker
}

func NewAccountReconcileProducer(accountCfg config.Account, account *models.Account, registry registry.StaticClient, store tickstore_types.TickstoreClient, db *gorm.DB) actor.Producer {
	return func() actor.Actor {
		return NewAccountReconcile(accountCfg, account, registry, store, db)
	}
}

func NewAccountReconcile(accountCfg config.Account, account *models.Account, registry registry.StaticClient, store tickstore_types.TickstoreClient, db *gorm.DB) actor.Actor {
	return &AccountReconcile{
		accountCfg: accountCfg,
		account:    account,
		store:      store,
		db:         db,
		registry:   registry,
	}
}

func (state *AccountReconcile) GetLogger() *log.Logger {
	return state.logger
}

func (state *AccountReconcile) Receive(context actor.Context) {
	extypes.ReconcileReceive(state, context)
}

func (state *AccountReconcile) Initialize(context actor.Context) error {
	// When initialize is done, the account must be aware of all the settings / assets / portfolio
	// so as to be able to answer to FIX messages

	state.logger = log.New(
		log.InfoLevel,
		"",
		log.String("ID", context.Self().Id),
		log.String("type", reflect.TypeOf(*state).String()))
	state.executor = actor.NewPID(context.ActorSystem().Address(), "executor/exchanges/"+constants.FBINANCE.Name+"_executor")

	// Request securities
	res, err := state.registry.Securities(goContext.Background(), &registry.SecuritiesRequest{
		Filter: &registry.SecurityFilter{
			ExchangeId: []uint32{constants.FBINANCE.ID},
		},
	})
	if err != nil {
		return fmt.Errorf("error fetching historical securities: %v", err)
	}

	state.symbToSecs = make(map[string]*registry.Security)
	securityMap := make(map[uint64]*registry.Security)
	for _, sec := range res.Securities {
		if strings.Contains(sec.Symbol, "SETTLED") {
			continue
		}
		securityMap[sec.SecurityId] = sec
		state.symbToSecs[sec.Symbol] = sec
	}
	state.securities = securityMap
	state.lastTradeID = make(map[uint64]uint64)

	// Start reconciliation
	state.positions = make(map[uint64]*account.Position)
	state.lastTradeID = make(map[uint64]uint64)
	for _, sec := range state.securities {
		state.positions[sec.SecurityId] = account.NewPosition(sec.SecurityId,
			sec.IsInverse, 1e8, 1e8, 1e8, 1, 0, 0)

		state.lastTradeID[sec.SecurityId] = 0
	}

	state.dbAccount = &extypes.Account{
		Name:       state.account.Name,
		ExchangeID: state.account.Exchange.ID,
	}
	// Check if account exists
	tx := state.db.Where("name=?", state.account.Name).FirstOrCreate(state.dbAccount)
	if tx.Error != nil {
		return fmt.Errorf("error creating account: %v", err)
	}

	if err := state.OnReconcile(context); err != nil {
		return fmt.Errorf("error reconciling: %v", err)
	}
	return nil
}

// TODO
func (state *AccountReconcile) Clean(context actor.Context) error {
	return nil
}

func (state *AccountReconcile) OnReconcile(context actor.Context) error {
	cutoff := time.Now()
	if err := state.reconcileMovements(context); err != nil {
		return fmt.Errorf("error reconcile movements: %v", err)
	}
	if err := state.reconcileTrades(context); err != nil {
		return fmt.Errorf("error reconcile trade: %v", err)
	}
	if err := state.saveToStore(context, cutoff); err != nil {
		return fmt.Errorf("error saving to store: %v", err)
	}
	return nil
}

func (state *AccountReconcile) OnAccountMovementRequest(context actor.Context) error {
	return nil
}

func (state *AccountReconcile) saveToStore(context actor.Context, cutoff time.Time) error {
	// Fetch balances
	resp, err := context.RequestFuture(state.executor, &messages.BalancesRequest{
		Account: state.account,
	}, 10*time.Second).Result()
	if err != nil {
		return fmt.Errorf("error getting positions from executor: %v", err)
	}
	balanceList, ok := resp.(*messages.BalanceList)
	if !ok {
		return fmt.Errorf("was expecting *messages.BalanceList, got %s", reflect.TypeOf(resp).String())
	}
	if !balanceList.Success {
		return fmt.Errorf("error getting balances: %s", balanceList.RejectionReason.String())
	}

	var movements []*extypes.Movement
	assets := make(map[uint32]float64)
	state.db.Debug().Model(&extypes.Movement{}).Joins("Transaction").Where(`"movements"."account_id"=? and time < ?`, state.dbAccount.ID, cutoff).Order("time asc").Find(&movements)
	for _, m := range movements {
		assets[m.AssetID] += m.Quantity
	}
	// Compare with balance list
	for _, bal := range balanceList.Balances {
		if assets[bal.Asset.ID] != bal.Quantity {
			fmt.Println("DIFFERENT BALANCE", bal.Asset.Symbol, assets[bal.Asset.ID], bal.Quantity)
			//return fmt.Errorf("different balance")
		}
	}

	fmt.Println("SAVING TO STORE")

	if state.store != nil {
		accountedTrades := make(map[uint]bool)
		assets := make(map[uint32]float64)
		prices := make(map[uint32]float64)
		prices[0] = 1
		tags := map[string]string{"account": state.account.Name}
		lastPortfolioEventTime, err := state.store.GetLastEventTime("portfolio",
			map[string]string{"account": "^" + state.account.Name + "$"})
		var writer tickstore_types.TickstoreWriter
		fmt.Println("LAST EVENT TIME", lastPortfolioEventTime)
		tracker := portfolio.NewPortfolioTracker()
		var deltas []portfolio.PortfolioTrackerDelta
		var lastTick = uint64(movements[0].Transaction.Time.UnixMilli())
		for _, m := range movements {
			tick := uint64(m.Transaction.Time.UnixMilli())
			if tick > lastTick {
				tickDelta := gotickfile.TickDeltas{
					Pointer: unsafe.Pointer(&deltas[0]),
					Len:     len(deltas),
				}
				if err := tracker.ProcessDeltas(tickDelta); err != nil {
					return fmt.Errorf("error applying delta: %v", err)
				}
				if tick > lastPortfolioEventTime {
					if writer == nil {
						writer, err = state.store.NewTickWriter("portfolio", tags, time.Second)
						if err != nil {
							return fmt.Errorf("error creating portfolio writer: %v", err)
						}
						if err := writer.WriteObject(lastTick, tracker); err != nil {
							return fmt.Errorf("error writing portfolio: %v", err)
						}
					} else {
						// Need to write deltas otherwise, always discontinuous portfolio (DeltasTo is discontinuous)
						if err := writer.WriteDeltas(lastTick, tickDelta); err != nil {
							return fmt.Errorf("error writing portfolio: %v", err)
						}
					}
					fmt.Println("WRITE", lastTick)
				}
				deltas = nil
			}
			switch messages.AccountMovementType(m.Reason) {
			case messages.AccountMovementType_Deposit, messages.AccountMovementType_Withdrawal:
				deltas = append(deltas, portfolio.NewTransferDelta(uint64(m.AssetID), m.Quantity))
			case messages.AccountMovementType_FundingFee:
				deltas = append(deltas, portfolio.NewFundingDelta(uint64(m.AssetID), m.Quantity))
			case messages.AccountMovementType_RealizedPnl:
				deltas = append(deltas, portfolio.NewRealizedPnLDelta(uint64(m.AssetID), m.Quantity))
			case messages.AccountMovementType_Commission:
				deltas = append(deltas, portfolio.NewCommissionDelta(uint64(m.AssetID), m.Quantity))
			case messages.AccountMovementType_WelcomeBonus:
				deltas = append(deltas, portfolio.NewWelcomeBonusDelta(uint64(m.AssetID), m.Quantity))
			}
			// If it's a trade, add a transfer
			if _, ok := accountedTrades[m.Transaction.ID]; !ok && m.Transaction.Type == "TRADE" {
				// Fetch fill
				var fill extypes.Fill
				if err := state.db.Model(&extypes.Fill{}).Where("transaction_id=?", m.Transaction.ID).First(&fill).Error; err != nil {
					return fmt.Errorf("error getting fill: %v", err)
				}
				sec := state.securities[uint64(fill.SecurityID)]
				quote, ok := constants.GetAssetBySymbol(sec.QuoteCurrency)
				if !ok {
					return fmt.Errorf("could not find quote asset %s", sec.QuoteCurrency)
				}
				deltas = append(deltas, portfolio.NewTradeDelta(uint64(quote.ID), uint64(fill.SecurityID), fill.Price, fill.Quantity))
				accountedTrades[m.Transaction.ID] = true
			}
			assets[m.AssetID] += m.Quantity
			lastTick = tick
		}
		if writer != nil {
			writer.Close()
		}
	}
	return nil
}

func (state *AccountReconcile) reconcileTrades(context actor.Context) error {
	var transactions []*extypes.Transaction
	state.db.Debug().Model(&extypes.Transaction{}).Joins("Fill").Where(`"transactions"."account_id"=?`, state.dbAccount.ID).Order("time asc, execution_id asc").Find(&transactions)
	for _, tr := range transactions {
		if tr.Type == "TRADE" {
			secID := uint64(tr.Fill.SecurityID)
			_, ok := state.securities[secID]
			if ok {
				if tr.Fill.Quantity < 0 {
					state.positions[secID].Sell(tr.Fill.Price, -tr.Fill.Quantity, false)
				} else {
					state.positions[secID].Buy(tr.Fill.Price, tr.Fill.Quantity, false)
				}
			}
			tradeID, err := strconv.ParseUint(strings.Split(tr.ExecutionID, "-")[0], 10, 64)
			if err != nil {
				return fmt.Errorf("error parsing executiong id: %v", err)
			}
			state.lastTradeID[secID] = tradeID
		}
	}

	// Fetch positions
	resp, err := context.RequestFuture(state.executor, &messages.PositionsRequest{
		Instrument: nil,
		Account:    state.account,
	}, 10*time.Second).Result()
	if err != nil {
		return fmt.Errorf("error getting positions from executor: %v", err)
	}

	positionList, ok := resp.(*messages.PositionList)
	if !ok {
		return fmt.Errorf("was expecting *messages.PositionList, got %s", reflect.TypeOf(resp).String())
	}
	if !positionList.Success {
		return fmt.Errorf("error getting positions: %s", positionList.RejectionReason.String())
	}

	end := time.Now()

	for _, sec := range state.securities {
		instrument := &models.Instrument{
			SecurityID: &wrapperspb.UInt64Value{Value: sec.SecurityId},
			Symbol:     &wrapperspb.StringValue{Value: sec.Symbol},
		}
		done := false
		for !done {
			fmt.Println("LAST", sec.SecurityId, state.lastTradeID[sec.SecurityId])
			res, err := context.RequestFuture(state.executor, &messages.TradeCaptureReportRequest{
				RequestID: 0,
				Filter: &messages.TradeCaptureReportFilter{
					FromID:     &wrapperspb.StringValue{Value: fmt.Sprintf("%d", state.lastTradeID[sec.SecurityId]+1)},
					Instrument: instrument,
				},
				Account: state.account,
			}, 20*time.Second).Result()
			if err != nil {
				fmt.Println("error getting trade capture report", err)
				time.Sleep(1 * time.Second)
				continue
			}
			trds := res.(*messages.TradeCaptureReport)
			if !trds.Success {
				fmt.Println("error getting trade capture report", trds.RejectionReason.String())
				time.Sleep(1 * time.Second)
				continue
			}
			progress := false
			for _, trd := range trds.Trades {
				ts := trd.TransactionTime.AsTime()
				if ts.After(end) {
					progress = false
					break
				}
				secID := trd.Instrument.SecurityID.Value
				var realized int64
				if trd.Quantity < 0 {
					_, realized = state.positions[secID].Sell(trd.Price, -trd.Quantity, false)
				} else {
					_, realized = state.positions[secID].Buy(trd.Price, trd.Quantity, false)
				}
				tr := &extypes.Transaction{
					Type:        "TRADE",
					Time:        ts,
					ExecutionID: trd.TradeID,
					AccountID:   state.dbAccount.ID,
					Fill: &extypes.Fill{
						AccountID:  state.dbAccount.ID,
						SecurityID: int64(secID),
						Price:      trd.Price,
						Quantity:   trd.Quantity,
					},
				}
				asset, ok := constants.GetAssetBySymbol(sec.QuoteCurrency)
				if !ok {
					return fmt.Errorf("error getting asset by symbol: %s", sec.QuoteCurrency)
				}
				// Realized PnL
				if realized != 0 {
					tr.Movements = append(tr.Movements, extypes.Movement{
						AccountID: state.dbAccount.ID,
						Reason:    int32(messages.AccountMovementType_RealizedPnl),
						AssetID:   asset.ID,
						Quantity:  -float64(realized) / 1e8,
					})
				}
				// Commission
				if trd.Commission != 0 {
					tr.Movements = append(tr.Movements, extypes.Movement{
						AccountID: state.dbAccount.ID,
						Reason:    int32(messages.AccountMovementType_Commission),
						AssetID:   trd.CommissionAsset.ID,
						Quantity:  -trd.Commission,
					})
				}

				tradeIDInt, _ := strconv.ParseUint(strings.Split(trd.TradeID, "-")[0], 10, 64)
				if tx := state.db.Create(tr); tx.Error != nil {
					return fmt.Errorf("error inserting transaction: %v", tx.Error)
				}
				state.lastTradeID[secID] = tradeIDInt
				progress = true
			}
			if len(trds.Trades) == 0 || !progress {
				done = true
			}
		}
	}

	state.positions = make(map[uint64]*account.Position)
	for _, sec := range state.securities {
		if sec.SecurityType == "CRPERP" {
			state.positions[sec.SecurityId] = account.NewPosition(sec.SecurityId,
				sec.IsInverse, 1e8, 1e8, 1e8, 1, 0, 0)
		}
	}
	state.db.Debug().
		Model(&extypes.Transaction{}).
		Joins("Fill").
		Where(`"transactions"."account_id"=?`, state.dbAccount.ID).
		Where(`"transactions"."time" < ?`, positionList.Time.AsTime()).
		Order("time asc, execution_id asc").
		Find(&transactions)
	for _, tr := range transactions {
		switch tr.Type {
		case "TRADE":
			if tr.Fill != nil {
				secID := uint64(tr.Fill.SecurityID)
				sec, ok := state.securities[secID]
				if ok && sec.SecurityType == "CRPERP" {
					if tr.Fill.Quantity < 0 {
						state.positions[secID].Sell(tr.Fill.Price, -tr.Fill.Quantity, false)
					} else {
						state.positions[secID].Buy(tr.Fill.Price, tr.Fill.Quantity, false)
					}
				}
			}
		}
	}

	execPositions := make(map[uint64]*models.Position)
	for _, pos := range positionList.Positions {
		execPositions[pos.Instrument.SecurityID.Value] = pos
	}

	for k, p1 := range state.positions {
		if p2, ok := execPositions[k]; ok {
			if p1.GetPosition() != nil {
				q1 := int(math.Round(1e8 * p1.GetPosition().Quantity))
				q2 := int(math.Round(1e8 * p2.Quantity))
				if q1 != q2 {
					return fmt.Errorf("different position quantity")
				}
			}
		} else if p1.GetPosition() != nil {
			return fmt.Errorf("%s not in exec", p1.GetPosition().Instrument.Symbol)
		}
	}

	return nil
}

func (state *AccountReconcile) reconcileMovements(context actor.Context) error {
	// Find funding transaction
	var cnt int64
	tx := state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "FUNDING").Count(&cnt)
	if tx.Error != nil {
		return fmt.Errorf("error getting funding transaction count: %v", tx.Error)
	}
	if cnt > 0 {
		var tr extypes.Transaction
		tx = state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "FUNDING").Order("time desc").First(&tr)
		if tx.Error != nil {
			return fmt.Errorf("error finding last funding transaction: %v", tx.Error)
		}
		state.lastFundingTs = uint64(tr.Time.UnixNano() / 1000000)
	} else {
		t, _ := time.Parse("2006-01-02", state.accountCfg.OpeningDate)
		state.lastFundingTs = uint64(t.UnixMilli())
	}

	tx = state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "DEPOSIT").Count(&cnt)
	if tx.Error != nil {
		return fmt.Errorf("error getting deposit transaction count: %v", tx.Error)
	}
	if cnt > 0 {
		var tr extypes.Transaction
		tx = state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "DEPOSIT").Order("time desc").First(&tr)
		if tx.Error != nil {
			return fmt.Errorf("error finding last deposit transaction: %v", tx.Error)
		}
		state.lastDepositTs = uint64(tr.Time.UnixNano() / 1000000)
	} else {
		t, _ := time.Parse("2006-01-02", state.accountCfg.OpeningDate)
		state.lastDepositTs = uint64(t.UnixMilli())
	}

	tx = state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "WITHDRAWAL").Count(&cnt)
	if tx.Error != nil {
		return fmt.Errorf("error getting withdrawal transaction count: %v", tx.Error)
	}
	if cnt > 0 {
		var tr extypes.Transaction
		tx = state.db.Model(&extypes.Transaction{}).Where("account_id=?", state.dbAccount.ID).Where("type=?", "WITHDRAWAL").Order("time desc").First(&tr)
		if tx.Error != nil {
			return fmt.Errorf("error finding last withdrawal transaction: %v", tx.Error)
		}
		state.lastWithdrawalTs = uint64(tr.Time.UnixNano() / 1000000)
	} else {
		t, _ := time.Parse("2006-01-02", state.accountCfg.OpeningDate)
		state.lastWithdrawalTs = uint64(t.UnixMilli())
	}

	// Get last account movement
	done := false
	for !done {
		res, err := context.RequestFuture(state.executor, &messages.AccountMovementRequest{
			RequestID: 0,
			Type:      messages.AccountMovementType_FundingFee,
			Filter: &messages.AccountMovementFilter{
				From: utils.MilliToTimestamp(state.lastFundingTs + 1),
				To:   utils.MilliToTimestamp(uint64(time.Now().UnixNano() / 1000000)),
			},
			Account: state.account,
		}, 20*time.Second).Result()
		if err != nil {
			fmt.Println("error getting movement", err)
			time.Sleep(1 * time.Second)
			continue
		}
		mvts := res.(*messages.AccountMovementResponse)
		if !mvts.Success {
			fmt.Println("error getting account movements", mvts.RejectionReason.String())
			time.Sleep(1 * time.Second)
			continue
		}
		progress := false
		for _, m := range mvts.Movements {
			ts := m.Time.AsTime()
			tr := &extypes.Transaction{
				Type:        "FUNDING",
				SubType:     m.Subtype,
				Time:        ts,
				ExecutionID: m.MovementID,
				AccountID:   state.dbAccount.ID,
				Fill:        nil,
				Movements: []extypes.Movement{{
					Reason:    int32(messages.AccountMovementType_FundingFee),
					AssetID:   m.Asset.ID,
					Quantity:  m.Change,
					AccountID: state.dbAccount.ID,
				}},
			}
			if tx := state.db.Create(tr); tx.Error != nil {
				return fmt.Errorf("error inserting funding: %v", tx.Error)
			}
			state.lastFundingTs = uint64(ts.UnixNano() / 1000000)
			progress = true
		}
		if len(mvts.Movements) == 0 || !progress {
			done = true
		}
	}

	done = false
	for !done {
		res, err := context.RequestFuture(state.executor, &messages.AccountMovementRequest{
			RequestID: 0,
			Type:      messages.AccountMovementType_Deposit,
			Filter: &messages.AccountMovementFilter{
				From: utils.MilliToTimestamp(state.lastDepositTs + 1),
				To:   utils.MilliToTimestamp(uint64(time.Now().UnixNano() / 1000000)),
			},
			Account: state.account,
		}, 20*time.Second).Result()
		if err != nil {
			fmt.Println("error getting movement", err)
			time.Sleep(1 * time.Second)
			continue
		}
		mvts := res.(*messages.AccountMovementResponse)
		if !mvts.Success {
			fmt.Println("error getting account movements", mvts.RejectionReason.String())
			time.Sleep(1 * time.Second)
			continue
		}
		progress := false
		for _, m := range mvts.Movements {
			ts := m.Time.AsTime()
			tr := &extypes.Transaction{
				Type:        "DEPOSIT",
				SubType:     m.Subtype,
				Time:        ts,
				ExecutionID: m.MovementID,
				AccountID:   state.dbAccount.ID,
				Fill:        nil,
				Movements: []extypes.Movement{{
					Reason:    int32(messages.AccountMovementType_Deposit),
					AssetID:   m.Asset.ID,
					Quantity:  m.Change,
					AccountID: state.dbAccount.ID,
				}},
			}
			if tx := state.db.Create(tr); tx.Error != nil {
				return fmt.Errorf("error inserting deposit: %v", tx.Error)
			}
			progress = true
			state.lastDepositTs = uint64(ts.UnixNano() / 1000000)
		}
		if len(mvts.Movements) == 0 || !progress {
			done = true
		}
	}

	done = false
	for !done {
		res, err := context.RequestFuture(state.executor, &messages.AccountMovementRequest{
			RequestID: 0,
			Type:      messages.AccountMovementType_Withdrawal,
			Filter: &messages.AccountMovementFilter{
				From: utils.MilliToTimestamp(state.lastWithdrawalTs + 1),
				To:   utils.MilliToTimestamp(uint64(time.Now().UnixNano() / 1000000)),
			},
			Account: state.account,
		}, 20*time.Second).Result()
		if err != nil {
			fmt.Println("error getting movement", err)
			time.Sleep(1 * time.Second)
			continue
		}
		mvts := res.(*messages.AccountMovementResponse)
		if !mvts.Success {
			fmt.Println("error getting account movements", mvts.RejectionReason.String())
			time.Sleep(1 * time.Second)
			continue
		}
		progress := false
		for _, m := range mvts.Movements {
			ts := m.Time.AsTime()
			tr := &extypes.Transaction{
				Type:        "WITHDRAWAL",
				SubType:     m.Subtype,
				Time:        ts,
				ExecutionID: m.MovementID,
				AccountID:   state.dbAccount.ID,
				Fill:        nil,
				Movements: []extypes.Movement{{
					Reason:    int32(messages.AccountMovementType_Withdrawal),
					AssetID:   m.Asset.ID,
					Quantity:  m.Change,
					AccountID: state.dbAccount.ID,
				}},
			}
			if tx := state.db.Create(tr); tx.Error != nil {
				return fmt.Errorf("error inserting withdrawal: %v", err)
			}
			progress = true
			state.lastWithdrawalTs = uint64(ts.UnixNano() / 1000000)
		}
		if len(mvts.Movements) == 0 || !progress {
			done = true
		}
	}

	return nil
}
