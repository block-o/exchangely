package ticker

type Ticker struct {
	Pair           string  `json:"pair"`
	Price          float64 `json:"price"`
	Variation24H   float64 `json:"variation_24h"`
	LastUpdateUnix int64   `json:"last_update_unix"`
	Source         string  `json:"source"`
}
