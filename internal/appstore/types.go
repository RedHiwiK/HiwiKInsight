package appstore

// NotificationPayload maps responseBodyV2DecodedPayload from Apple's documentation (only the fields in use).
type NotificationPayload struct {
	NotificationType string `json:"notificationType"`
	Subtype          string `json:"subtype"`
	NotificationUUID string `json:"notificationUUID"`
	SignedDate       int64  `json:"signedDate"`
	Data             struct {
		BundleID                 string `json:"bundleId"`
		Environment              string `json:"environment"`
		SignedTransactionInfo    string `json:"signedTransactionInfo"`
		SignedRenewalInfo        string `json:"signedRenewalInfo"`
		ConsumptionRequestReason string `json:"consumptionRequestReason"`
	} `json:"data"`
}

// TransactionInfo maps JWSTransactionDecodedPayload (only the fields in use).
type TransactionInfo struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	Type                  string `json:"type"`
	Environment           string `json:"environment"`
	Storefront            string `json:"storefront"`
	Currency              string `json:"currency"`
	Price                 int64  `json:"price"` // in thousandths of the currency unit
	PurchaseDate          int64  `json:"purchaseDate"`
	OriginalPurchaseDate  int64  `json:"originalPurchaseDate"`
	ExpiresDate           int64  `json:"expiresDate"`
	TransactionReason     string `json:"transactionReason"`  // PURCHASE / RENEWAL
	InAppOwnershipType    string `json:"inAppOwnershipType"` // PURCHASED / FAMILY_SHARED
	OfferType             int    `json:"offerType"`
	OfferDiscountType     string `json:"offerDiscountType"`
	OfferIdentifier       string `json:"offerIdentifier"`
	RevocationDate        int64  `json:"revocationDate"`
	RevocationReason      *int   `json:"revocationReason"`
	AppAccountToken       string `json:"appAccountToken"` // UUID passed by the app at purchase time, used to link purchase.started
}

// RenewalInfo maps JWSRenewalInfoDecodedPayload (only present for auto-renewable subscriptions).
type RenewalInfo struct {
	AutoRenewStatus        int    `json:"autoRenewStatus"` // 1 on / 0 off
	AutoRenewProductID     string `json:"autoRenewProductId"`
	ExpirationIntent       int    `json:"expirationIntent"`
	GracePeriodExpiresDate int64  `json:"gracePeriodExpiresDate"`
	IsInBillingRetryPeriod bool   `json:"isInBillingRetryPeriod"`
	RenewalPrice           int64  `json:"renewalPrice"` // in thousandths of the currency unit
	Currency               string `json:"currency"`
}
