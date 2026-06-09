package value_object

// TenantFeatures holds the feature flags that auth embeds in the JWT.
// This is auth's own type — it does not import the tenant module.
// The conversion from tenant's TenantFeatures happens in the infrastructure adapter.
type TenantFeatures struct {
	FriendsFamily    bool `json:"friends_family"`
	PremiumAnalytics bool `json:"premium_analytics"`
}

func DefaultTenantFeatures() *TenantFeatures {
	return &TenantFeatures{}
}
