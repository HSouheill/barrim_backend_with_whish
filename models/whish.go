package models

// WhishRequest represents the standard request structure for Whish API
type WhishRequest struct {
	Amount             *float64 `json:"amount,omitempty"`
	Currency           string   `json:"currency,omitempty"`
	Invoice            string   `json:"invoice,omitempty"`
	ExternalID         *int64   `json:"externalId,omitempty"`
	SuccessCallbackURL string   `json:"successCallbackUrl,omitempty"`
	FailureCallbackURL string   `json:"failureCallbackUrl,omitempty"`
	SuccessRedirectURL string   `json:"successRedirectUrl,omitempty"`
	FailureRedirectURL string   `json:"failureRedirectUrl,omitempty"`
}

// WhishResponse represents the standard response structure from Whish API
type WhishResponse struct {
	Status bool                   `json:"status"`
	Code   interface{}            `json:"code"`   // Can be string or null
	Dialog interface{}            `json:"dialog"` // Can be string, object, or null
	Extra  interface{}            `json:"extra"`
	Data   map[string]interface{} `json:"data"`
}

// BalanceDetails represents the balance information
type BalanceDetails struct {
	Balance float64 `json:"balance"`
}

// RateData represents the rate information
type RateData struct {
	Rate float64 `json:"rate"`
}

// CollectURLData represents the payment collection URL
type CollectURLData struct {
	CollectURL string `json:"collectUrl"`
}

// PaymentStatusData represents the payment status information
type PaymentStatusData struct {
	CollectStatus    string `json:"collectStatus"`
	PayerPhoneNumber string `json:"payerPhoneNumber"`
}
