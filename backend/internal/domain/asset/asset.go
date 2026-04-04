package asset

type Asset struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	CirculatingSupply float64 `json:"circulating_supply"`
}
