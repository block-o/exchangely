package ticker

type Ticker struct {
	Pair           string  `json:"pair"`
	Price          float64 `json:"price"`
	MarketCap      float64 `json:"market_cap"`
	Variation24H   float64 `json:"variation_24h"`
	High24H        float64 `json:"high_24h"`
	Low24H         float64 `json:"low_24h"`
	LastUpdateUnix int64   `json:"last_update_unix"`
	Source         string  `json:"source"`
}
