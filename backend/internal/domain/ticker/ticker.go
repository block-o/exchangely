package ticker

// Ticker represents a point-in-time snapshot of a trading pair's market state,
// including price, market capitalization, and historical variations.
type Ticker struct {
	Pair  string  `json:"pair"`
	Price float64 `json:"price"`

	// MarketCap is calculated as Price * CirculatingSupply.
	MarketCap float64 `json:"market_cap"`

	// Variation metrics represent percentage change over historical baselines.
	Variation1H  float64 `json:"variation_1h"`
	Variation24H float64 `json:"variation_24h"`
	Variation7D  float64 `json:"variation_7d"`

	// Volume24H is the sum of traded volume over the trailing 24 hours.
	Volume24H float64 `json:"volume_24h"`

	// 24h Price bounds.
	High24H float64 `json:"high_24h"`
	Low24H  float64 `json:"low_24h"`

	// Metadata.
	LastUpdateUnix int64  `json:"last_update_unix"`
	Source         string `json:"source"`
}
