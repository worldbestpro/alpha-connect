package modeling

import (
	"math"
	"math/rand"
)

type Model interface {
	GetSecurityPrice(securityID uint64) float64
	GetAssetPrice(assetID uint32) float64
	GetSampleAssetPrices(assetID uint32, time uint64, sampleSize int) []float64
	GetSampleSecurityPrices(securityID uint64, time uint64, sampleSize int) []float64
	GetSampleMatchBid(securityID uint64, time uint64, sampleSize int) []float64
	GetSampleMatchAsk(securityID uint64, time uint64, sampleSize int) []float64
}

type MapModel struct {
	buyTradeModels      map[uint64]BuyTradeModel
	sellTradeModels     map[uint64]SellTradeModel
	securityPriceModels map[uint64]PriceModel
	assetPriceModels    map[uint32]PriceModel
}

func NewMapModel() *MapModel {
	return &MapModel{
		buyTradeModels:      make(map[uint64]BuyTradeModel),
		sellTradeModels:     make(map[uint64]SellTradeModel),
		securityPriceModels: make(map[uint64]PriceModel),
		assetPriceModels:    make(map[uint32]PriceModel),
	}
}

func (m *MapModel) GetSecurityPrice(securityID uint64) float64 {
	return m.securityPriceModels[securityID].GetPrice()
}

func (m *MapModel) GetAssetPrice(assetID uint32) float64 {
	return m.assetPriceModels[assetID].GetPrice()
}

func (m *MapModel) GetSampleAssetPrices(assetID uint32, time uint64, sampleSize int) []float64 {
	return m.assetPriceModels[assetID].GetSamplePrices(time, sampleSize)
}

func (m *MapModel) GetSampleSecurityPrices(securityID uint64, time uint64, sampleSize int) []float64 {
	return m.securityPriceModels[securityID].GetSamplePrices(time, sampleSize)
}

func (m *MapModel) GetSampleMatchBid(securityID uint64, time uint64, sampleSize int) []float64 {
	return m.sellTradeModels[securityID].GetSampleMatchBid(time, sampleSize)
}

func (m *MapModel) GetSampleMatchAsk(securityID uint64, time uint64, sampleSize int) []float64 {
	return m.buyTradeModels[securityID].GetSampleMatchAsk(time, sampleSize)
}

func (m *MapModel) SetSecurityPriceModel(securityID uint64, model PriceModel) {
	m.securityPriceModels[securityID] = model
}

func (m *MapModel) SetAssetPriceModel(assetID uint32, model PriceModel) {
	m.assetPriceModels[assetID] = model
}

func (m *MapModel) SetBuyTradeModel(securityID uint64, model BuyTradeModel) {
	m.buyTradeModels[securityID] = model
}

func (m *MapModel) SetSellTradeModel(securityID uint64, model SellTradeModel) {
	m.sellTradeModels[securityID] = model
}

type PriceModel interface {
	Update(feedID uint64, tick uint64, price float64)
	Progress(tick uint64)
	GetPrice() float64
	GetSamplePrices(time uint64, sampleSize int) []float64
	Frequency() uint64
}

type TradeModel interface {
	Update(feedID uint64, tick uint64, size float64)
	Progress(tick uint64)
}

type SellTradeModel interface {
	TradeModel
	GetSampleMatchBid(time uint64, sampleSize int) []float64
}

type BuyTradeModel interface {
	TradeModel
	GetSampleMatchAsk(time uint64, sampleSize int) []float64
}

type ConstantPriceModel struct {
	price        float64
	samplePrices []float64
}

func NewConstantPriceModel(price float64) PriceModel {
	return &ConstantPriceModel{
		price:        price,
		samplePrices: nil,
	}
}

func (m *ConstantPriceModel) Update(_ uint64, _ uint64, _ float64) {

}

func (m *ConstantPriceModel) Progress(_ uint64) {

}

func (m *ConstantPriceModel) Frequency() uint64 {
	return 0
}

func (m *ConstantPriceModel) GetSamplePrices(time uint64, sampleSize int) []float64 {
	if m.samplePrices == nil || len(m.samplePrices) != sampleSize {
		m.samplePrices = make([]float64, sampleSize, sampleSize)
		for i := 0; i < sampleSize; i++ {
			m.samplePrices[i] = m.price
		}
	}
	return m.samplePrices
}

func (m *ConstantPriceModel) GetPrice() float64 {
	return m.price
}

type GBMPriceModel struct {
	time         uint64
	price        float64
	freq         uint64
	samplePrices []float64
	sampleTime   uint64
}

func NewGBMPriceModel(price float64, freq uint64) PriceModel {
	return &GBMPriceModel{
		time:         0,
		price:        price,
		freq:         freq,
		samplePrices: nil,
		sampleTime:   0,
	}
}

func (m *GBMPriceModel) Update(_ uint64, _ uint64, _ float64) {

}

func (m *GBMPriceModel) Progress(time uint64) {
	for m.time < time {
		m.price *= math.Exp(rand.NormFloat64())
		m.time += m.freq
	}
}

func (m *GBMPriceModel) Frequency() uint64 {
	return m.freq
}

func (m *GBMPriceModel) GetSamplePrices(time uint64, sampleSize int) []float64 {
	if m.samplePrices == nil || len(m.samplePrices) != sampleSize || m.sampleTime != time {
		intervalLength := int((time - m.time) / m.freq)
		m.samplePrices = make([]float64, sampleSize, sampleSize)
		for i := 0; i < sampleSize; i++ {
			m.samplePrices[i] = m.price
			for j := 0; j < intervalLength; j++ {
				m.samplePrices[i] *= (rand.NormFloat64() / 10) + 1
			}
		}
		m.sampleTime = time
	}
	return m.samplePrices
}

func (m *GBMPriceModel) GetPrice() float64 {
	return m.price
}

type ConstantTradeModel struct {
	match       float64
	sampleMatch []float64
}

func NewConstantTradeModel(match float64) *ConstantTradeModel {
	return &ConstantTradeModel{
		match:       match,
		sampleMatch: nil,
	}
}

func (m *ConstantTradeModel) Update(_ uint64, _ uint64, _ float64) {

}

func (m *ConstantTradeModel) Progress(_ uint64) {

}

func (m *ConstantTradeModel) GetSampleMatchAsk(time uint64, sampleSize int) []float64 {
	if m.sampleMatch == nil || len(m.sampleMatch) != sampleSize {
		m.sampleMatch = make([]float64, sampleSize, sampleSize)
		for i := 0; i < sampleSize; i++ {
			m.sampleMatch[i] = m.match
		}
	}
	return m.sampleMatch
}

func (m *ConstantTradeModel) GetSampleMatchBid(time uint64, sampleSize int) []float64 {
	if m.sampleMatch == nil || len(m.sampleMatch) != sampleSize {
		m.sampleMatch = make([]float64, sampleSize, sampleSize)
		for i := 0; i < sampleSize; i++ {
			m.sampleMatch[i] = m.match
		}
	}
	return m.sampleMatch
}