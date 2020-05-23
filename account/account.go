package account

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	"gitlab.com/alphaticks/alphac/enum"
	"gitlab.com/alphaticks/alphac/models"
	"gitlab.com/alphaticks/alphac/models/messages"
	xchangerModels "gitlab.com/alphaticks/xchanger/models"
	"math"
)

type Position struct {
	Cost    int64
	RawSize int64
	Cross   bool
}

type Security struct {
	*models.Security
	Position      *Position
	TickPrecision float64
	LotPrecision  float64
}

type Order struct {
	*models.Order
	previousStatus models.OrderStatus
}

type Account struct {
	ID              string
	ordersID        map[string]*Order
	ordersClID      map[string]*Order
	securities      map[uint64]*Security
	balances        map[uint32]float64
	margin          int64
	marginCurrency  uint32
	marginPrecision float64
}

func NewAccount(ID string, securities []*models.Security, marginCurrency *xchangerModels.Asset, marginPrecision float64) *Account {
	accnt := &Account{
		ID:              ID,
		ordersID:        make(map[string]*Order),
		ordersClID:      make(map[string]*Order),
		securities:      make(map[uint64]*Security),
		balances:        make(map[uint32]float64),
		margin:          0,
		marginCurrency:  marginCurrency.ID,
		marginPrecision: marginPrecision,
	}
	for _, s := range securities {
		accnt.securities[s.SecurityID] = &Security{
			Security:      s,
			TickPrecision: math.Ceil(1. / s.MinPriceIncrement),
			LotPrecision:  math.Ceil(1. / s.RoundLot),
			Position: &Position{
				Cost:    0.,
				RawSize: 0,
				Cross:   true,
			},
		}
	}

	return accnt
}

func (accnt *Account) Sync(orders []*models.Order, positions []*models.Position, balances []*models.Balance) error {

	for _, o := range orders {
		ord := &Order{
			Order:          o,
			previousStatus: o.OrderStatus,
		}
		accnt.ordersID[o.OrderID] = ord
		accnt.ordersClID[o.ClientOrderID] = ord
	}

	// Reset securities
	for _, s := range accnt.securities {
		s.Position.Cost = 0.
		s.Position.RawSize = 0
	}
	for _, p := range positions {
		if p.AccountID != accnt.ID {
			return fmt.Errorf("got position for wrong account ID")
		}
		if p.Instrument == nil {
			return fmt.Errorf("position with nil instrument")
		}
		if p.Instrument.SecurityID == nil {
			return fmt.Errorf("position with nil security ID")
		}
		sec, ok := accnt.securities[p.Instrument.SecurityID.Value]
		if !ok {
			return fmt.Errorf("security %d for order not found", p.Instrument.SecurityID.Value)
		}
		rawSize := int64(math.Round(sec.LotPrecision * p.Quantity))
		sec.Position.RawSize = rawSize
		sec.Position.Cross = p.Cross
		sec.Position.Cost = int64(p.Cost * accnt.marginPrecision)
	}

	for _, b := range balances {
		accnt.balances[b.Asset.ID] = b.Quantity
	}

	return nil
}

func (accnt *Account) NewOrder(order *models.Order) (*messages.ExecutionReport, *messages.RejectionReason) {
	if _, ok := accnt.ordersClID[order.ClientOrderID]; ok {
		res := messages.DuplicateOrder
		return nil, &res
	}
	if order.OrderStatus != models.PendingNew {
		res := messages.Other
		return nil, &res
	}
	if order.Instrument == nil || order.Instrument.SecurityID == nil {
		res := messages.UnknownSymbol
		return nil, &res
	}
	sec, ok := accnt.securities[order.Instrument.SecurityID.Value]
	if !ok {
		res := messages.UnknownSymbol
		return nil, &res
	}
	rawLeavesQuantity := sec.LotPrecision * order.LeavesQuantity
	if math.Abs(rawLeavesQuantity-math.Round(rawLeavesQuantity)) > 0.00001 {
		res := messages.IncorrectQuantity
		return nil, &res
	}
	rawCumQty := int64(order.CumQuantity * sec.LotPrecision)
	if rawCumQty > 0 {
		res := messages.IncorrectQuantity
		return nil, &res
	}
	accnt.ordersClID[order.ClientOrderID] = &Order{
		Order:          order,
		previousStatus: order.OrderStatus,
	}
	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.PendingNew,
		OrderStatus:     order.OrderStatus,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
	}, nil
}

func (accnt *Account) ConfirmNewOrder(clientID string, ID string) (*messages.ExecutionReport, error) {
	order, ok := accnt.ordersClID[clientID]
	if !ok {
		return nil, fmt.Errorf("unknown order %s", clientID)
	}
	if order.OrderStatus != models.PendingNew {
		// Order already confirmed, nop
		return nil, nil
	}
	order.OrderID = ID
	order.OrderStatus = models.New
	accnt.ordersID[ID] = order
	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.New,
		OrderStatus:     order.OrderStatus,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
	}, nil
}

func (accnt *Account) RejectNewOrder(clientID string, reason messages.RejectionReason) (*messages.ExecutionReport, error) {
	order, ok := accnt.ordersClID[clientID]
	if !ok {
		return nil, fmt.Errorf("unknown order %s", clientID)
	}
	order.OrderStatus = models.Rejected
	delete(accnt.ordersClID, clientID)

	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.Rejected,
		OrderStatus:     models.Rejected,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
		RejectionReason: reason,
	}, nil
}

func (accnt *Account) CancelOrder(ID string) (*messages.ExecutionReport, *messages.RejectionReason) {
	var order *Order
	order, _ = accnt.ordersClID[ID]
	if order == nil {
		order, _ = accnt.ordersID[ID]
	}
	if order == nil {
		res := messages.UnknownOrder
		return nil, &res
	}
	if order.OrderStatus == models.PendingCancel {
		res := messages.CancelAlreadyPending
		return nil, &res
	}

	// Save current order status in case cancel gets rejected
	order.previousStatus = order.OrderStatus
	order.OrderStatus = models.PendingCancel

	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.PendingCancel,
		OrderStatus:     models.PendingCancel,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
	}, nil
}

func (accnt *Account) GetOrders(filter *messages.OrderFilter) []*models.Order {
	var orders []*models.Order
	for _, o := range accnt.ordersClID {
		if filter != nil && filter.Instrument != nil && o.Instrument.SecurityID.Value != filter.Instrument.SecurityID.Value {
			continue
		}
		if filter != nil && filter.Side != nil && o.Side != filter.Side.Value {
			continue
		}
		if filter != nil && filter.OrderID != nil && o.OrderID != filter.OrderID.Value {
			continue
		}
		if filter != nil && filter.ClientOrderID != nil && o.ClientOrderID != filter.ClientOrderID.Value {
			continue
		}
		if filter != nil && filter.OrderStatus != nil && o.OrderStatus != filter.OrderStatus.Value {
			continue
		}
		orders = append(orders, o.Order)
	}

	return orders
}

func (accnt *Account) ConfirmCancelOrder(ID string) (*messages.ExecutionReport, error) {
	var order *Order
	order, _ = accnt.ordersClID[ID]
	if order == nil {
		order, _ = accnt.ordersID[ID]
	}
	if order == nil {
		return nil, fmt.Errorf("unknown order %s", ID)
	}
	if order.OrderStatus == models.Canceled {
		return nil, nil
	}
	if order.OrderStatus != models.PendingCancel {
		return nil, fmt.Errorf("error not pending cancel")
	}

	order.OrderStatus = models.Canceled
	order.LeavesQuantity = 0.

	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.Canceled,
		OrderStatus:     models.Canceled,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
	}, nil
}

func (accnt *Account) RejectCancelOrder(ID string, reason messages.RejectionReason) (*messages.ExecutionReport, error) {
	var order *Order
	order, _ = accnt.ordersClID[ID]
	if order == nil {
		order, _ = accnt.ordersID[ID]
	}
	if order == nil {
		return nil, fmt.Errorf("unknown order %s", ID)
	}
	order.OrderStatus = order.previousStatus

	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.Rejected,
		OrderStatus:     order.OrderStatus,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
		RejectionReason: reason,
	}, nil
}

func (accnt *Account) ConfirmFill(ID string, tradeID string, price, quantity float64, taker bool) (*messages.ExecutionReport, error) {
	var order *Order
	order, _ = accnt.ordersClID[ID]
	if order == nil {
		order, _ = accnt.ordersID[ID]
	}
	if order == nil {
		return nil, fmt.Errorf("unknown order %s", ID)
	}
	sec := accnt.securities[order.Instrument.SecurityID.Value]
	rawFillQuantity := int64(quantity * sec.LotPrecision)
	rawLeavesQuantity := int64(order.LeavesQuantity * sec.LotPrecision)
	if rawFillQuantity > rawLeavesQuantity {
		return nil, fmt.Errorf("fill bigger than order leaves quantity")
	}
	rawCumQuantity := int64(order.CumQuantity * sec.LotPrecision)
	order.LeavesQuantity = float64(rawLeavesQuantity-rawFillQuantity) / sec.LotPrecision
	order.CumQuantity = float64(rawCumQuantity+rawFillQuantity) / sec.LotPrecision
	if rawFillQuantity == rawLeavesQuantity {
		order.OrderStatus = models.Filled
	} else {
		order.OrderStatus = models.PartiallyFilled
	}

	switch sec.SecurityType {
	case enum.SecurityType_CRYPTO_SPOT:
		if order.Side == models.Buy {
			accnt.balances[sec.Underlying.ID] += quantity
			accnt.balances[sec.QuoteCurrency.ID] -= quantity * price
		} else {
			accnt.balances[sec.Underlying.ID] -= quantity
			accnt.balances[sec.QuoteCurrency.ID] += quantity * price
		}
	case enum.SecurityType_CRYPTO_PERP:
		if sec.Position == nil {
			sec.Position = &Position{
				Cost:    0,
				RawSize: 0,
				Cross:   true,
			}
		}
		if order.Side == models.Buy {
			accnt.Buy(sec, price, quantity, taker)
		} else {
			accnt.Sell(sec, price, quantity, taker)
		}
	}

	return &messages.ExecutionReport{
		OrderID:         order.OrderID,
		ClientOrderID:   &types.StringValue{Value: order.ClientOrderID},
		ExecutionID:     "", // TODO
		ExecutionType:   messages.Trade,
		OrderStatus:     order.OrderStatus,
		Instrument:      order.Instrument,
		LeavesQuantity:  order.LeavesQuantity,
		CumQuantity:     order.CumQuantity,
		TransactionTime: types.TimestampNow(),
		TradeID:         &types.StringValue{Value: tradeID},
		FillPrice:       &types.DoubleValue{Value: price},
		FillQuantity:    &types.DoubleValue{Value: quantity},
	}, nil
}

func (accnt *Account) Buy(sec *Security, price, quantity float64, taker bool) {
	if sec.IsInverse {
		price = 1. / price
	}
	rawFillQuantity := int64(quantity * sec.LotPrecision)
	if sec.Position.RawSize < 0 {
		// We are closing our positions from c.size to c.size + size
		closedSize := rawFillQuantity
		// we don't close 'size' if we go over 0 and re-open longs
		if -sec.Position.RawSize < closedSize {
			closedSize = -sec.Position.RawSize
		}

		contractMarginValue := int64((float64(sec.Position.RawSize) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)
		unrealizedCost := sec.Position.Cost - contractMarginValue
		realizedCost := (closedSize * unrealizedCost) / sec.Position.RawSize

		closedMarginValue := int64((float64(closedSize) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)

		// Remove closed from cost
		sec.Position.Cost += closedMarginValue
		// Remove realized cost
		sec.Position.Cost += realizedCost

		accnt.margin += realizedCost

		rawFillQuantity -= closedSize
		sec.Position.RawSize += closedSize
	}
	if rawFillQuantity > 0 {
		// We are opening a position
		openedMarginValue := int64((float64(rawFillQuantity) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)
		sec.Position.Cost += openedMarginValue
		sec.Position.RawSize += rawFillQuantity
	}
}

func (accnt *Account) Sell(sec *Security, price, quantity float64, taker bool) {
	if sec.IsInverse {
		price = 1. / price
	}
	rawFillQuantity := int64(quantity * sec.LotPrecision)
	if sec.Position.RawSize > 0 {
		// We are closing our position from c.size to c.size + size
		closedSize := rawFillQuantity
		if sec.Position.RawSize < closedSize {
			closedSize = sec.Position.RawSize
		}
		contractMarginValue := int64((float64(sec.Position.RawSize) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)
		closedMarginValue := int64((float64(closedSize) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)

		unrealizedCost := sec.Position.Cost - contractMarginValue
		realizedCost := (closedSize * unrealizedCost) / sec.Position.RawSize

		// Transfer cost
		sec.Position.Cost -= closedMarginValue
		// Remove realized cost
		sec.Position.Cost -= realizedCost
		sec.Position.RawSize -= closedSize
		accnt.margin -= realizedCost
		rawFillQuantity -= closedSize
	}
	if rawFillQuantity > 0 {
		// We are opening a position
		openedMarginValue := int64((float64(rawFillQuantity) / sec.LotPrecision) * price * sec.Multiplier.Value * accnt.marginPrecision)
		sec.Position.Cost -= openedMarginValue
		sec.Position.RawSize -= rawFillQuantity
	}
}

func (accnt *Account) GetPositions() []*models.Position {
	var positions []*models.Position
	for _, s := range accnt.securities {
		if s.Position.RawSize != 0 {
			positions = append(positions,
				&models.Position{
					AccountID: accnt.ID,
					Instrument: &models.Instrument{
						SecurityID: &types.UInt64Value{Value: s.SecurityID},
						Exchange:   s.Exchange,
						Symbol:     &types.StringValue{Value: s.Symbol},
					},
					Quantity: float64(s.Position.RawSize) / s.LotPrecision,
					Cost:     float64(s.Position.Cost) / accnt.marginPrecision,
					Cross:    s.Position.Cross,
				})
		}
	}

	return positions
}

func (accnt *Account) GetMargin() float64 {
	return float64(accnt.margin) / accnt.marginPrecision
}
