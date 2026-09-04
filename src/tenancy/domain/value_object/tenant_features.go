package value_object

// TenantFeatures representa las características habilitadas para un tenant
type TenantFeatures struct {
	FriendsFamily    bool `json:"friends_family"`
	PremiumAnalytics bool `json:"premium_analytics"`
}

// NewTenantFeatures crea una nueva instancia con valores por defecto
func NewTenantFeatures() *TenantFeatures {
	return &TenantFeatures{
		FriendsFamily:    false,
		PremiumAnalytics: false,
	}
}

// NewTenantFeaturesWithValues crea una nueva instancia con valores específicos
func NewTenantFeaturesWithValues(friendsFamily, premiumAnalytics bool) *TenantFeatures {
	return &TenantFeatures{
		FriendsFamily:    friendsFamily,
		PremiumAnalytics: premiumAnalytics,
	}
}

// UpdateFriendsFamily actualiza el feature flag de friends & family
func (tf *TenantFeatures) UpdateFriendsFamily(enabled bool) {
	tf.FriendsFamily = enabled
}

// UpdatePremiumAnalytics actualiza el feature flag de premium analytics
func (tf *TenantFeatures) UpdatePremiumAnalytics(enabled bool) {
	tf.PremiumAnalytics = enabled
}

// HasFriendsFamily verifica si el feature friends & family está habilitado
func (tf *TenantFeatures) HasFriendsFamily() bool {
	return tf.FriendsFamily
}

// HasPremiumAnalytics verifica si el feature premium analytics está habilitado
func (tf *TenantFeatures) HasPremiumAnalytics() bool {
	return tf.PremiumAnalytics
}

// ToMap convierte los features a un mapa para serialización
func (tf *TenantFeatures) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"friends_family":    tf.FriendsFamily,
		"premium_analytics": tf.PremiumAnalytics,
	}
}

// FromMap carga los features desde un mapa
func (tf *TenantFeatures) FromMap(data map[string]interface{}) {
	if val, ok := data["friends_family"].(bool); ok {
		tf.FriendsFamily = val
	}
	if val, ok := data["premium_analytics"].(bool); ok {
		tf.PremiumAnalytics = val
	}
}
