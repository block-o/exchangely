package candle

type Candle struct {
	Pair      string  `json:"pair"`
	Interval  string  `json:"interval"`
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	Volume24H float64 `json:"volume_24h,omitempty"`
	Source    string  `json:"source"`
	Finalized bool    `json:"finalized"`
}
