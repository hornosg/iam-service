package entity

import (
	"time"

	"iam/src/plan/domain/entity"
	"iam/src/plan/domain/value_object"

	"github.com/google/uuid"
)

// PlanMother implementa el patrón Object Mother para crear entities Plan de prueba
type PlanMother struct{}

// WithDefaults crea un plan con valores por defecto
func (PlanMother) WithDefaults() *entity.Plan {
	return entity.NewPlan(
		"Plan de Prueba",
		"Descripción del plan de prueba",
		value_object.PlanTypeBasic,
		9.99,
		99.99,
	)
}

// WithID crea un plan con un ID específico
func (p PlanMother) WithID(id uuid.UUID) *entity.Plan {
	plan := p.WithDefaults()
	plan.ID = id
	return plan
}

// WithName crea un plan con un nombre específico
func (p PlanMother) WithName(name string) *entity.Plan {
	plan := p.WithDefaults()
	plan.Name = name
	return plan
}

// WithType crea un plan con un tipo específico
func (p PlanMother) WithType(planType value_object.PlanType) *entity.Plan {
	plan := p.WithDefaults()
	plan.Type = planType
	plan.MaxUsers = planType.GetMaxUsers()
	return plan
}

// WithStatus crea un plan con un estado específico
func (p PlanMother) WithStatus(status value_object.PlanStatus) *entity.Plan {
	plan := p.WithDefaults()
	plan.Status = status
	return plan
}

// WithPricing crea un plan con precios específicos
func (p PlanMother) WithPricing(priceMonth, priceYear float64) *entity.Plan {
	plan := p.WithDefaults()
	plan.PriceMonth = priceMonth
	plan.PriceYear = priceYear
	return plan
}

// WithFeatures crea un plan con características específicas
func (p PlanMother) WithFeatures(features []string) *entity.Plan {
	plan := p.WithDefaults()
	plan.Features = make([]string, len(features))
	copy(plan.Features, features)
	return plan
}

// Free crea un plan gratuito
func (p PlanMother) Free() *entity.Plan {
	plan := entity.NewPlan(
		"Plan Gratuito",
		"Plan gratuito con funcionalidades básicas",
		value_object.PlanTypeFree,
		0.0,
		0.0,
	)
	plan.AddFeature("basic_dashboard")
	plan.AddFeature("1_user")
	plan.AddFeature("basic_support")
	return plan
}

// Basic crea un plan básico
func (p PlanMother) Basic() *entity.Plan {
	plan := entity.NewPlan(
		"Plan Básico",
		"Plan básico para pequeños equipos",
		value_object.PlanTypeBasic,
		9.99,
		99.99,
	)
	plan.AddFeature("advanced_dashboard")
	plan.AddFeature("up_to_10_users")
	plan.AddFeature("email_support")
	plan.AddFeature("basic_analytics")
	return plan
}

// Premium crea un plan premium
func (p PlanMother) Premium() *entity.Plan {
	plan := entity.NewPlan(
		"Plan Premium",
		"Plan premium para equipos en crecimiento",
		value_object.PlanTypePremium,
		29.99,
		299.99,
	)
	plan.AddFeature("full_dashboard")
	plan.AddFeature("up_to_100_users")
	plan.AddFeature("priority_support")
	plan.AddFeature("advanced_analytics")
	plan.AddFeature("api_access")
	plan.AddFeature("custom_integrations")
	return plan
}

// Enterprise crea un plan enterprise
func (p PlanMother) Enterprise() *entity.Plan {
	plan := entity.NewPlan(
		"Plan Enterprise",
		"Plan enterprise para grandes organizaciones",
		value_object.PlanTypeEnterprise,
		99.99,
		999.99,
	)
	plan.AddFeature("enterprise_dashboard")
	plan.AddFeature("unlimited_users")
	plan.AddFeature("dedicated_support")
	plan.AddFeature("enterprise_analytics")
	plan.AddFeature("full_api_access")
	plan.AddFeature("custom_integrations")
	plan.AddFeature("sso_integration")
	plan.AddFeature("audit_logs")
	plan.AddFeature("compliance_tools")
	return plan
}

// Inactive crea un plan inactivo
func (p PlanMother) Inactive() *entity.Plan {
	plan := p.WithDefaults()
	plan.ChangeStatus(value_object.PlanStatusInactive)
	return plan
}

// Deprecated crea un plan deprecado
func (p PlanMother) Deprecated() *entity.Plan {
	plan := p.WithDefaults()
	plan.ChangeStatus(value_object.PlanStatusDeprecated)
	return plan
}

// WithDescription crea un plan con una descripción específica
func (p PlanMother) WithDescription(description string) *entity.Plan {
	plan := p.WithDefaults()
	plan.Description = description
	return plan
}

// WithMaxUsers crea un plan con un límite específico de usuarios
func (p PlanMother) WithMaxUsers(maxUsers int) *entity.Plan {
	plan := p.WithDefaults()
	plan.MaxUsers = maxUsers
	return plan
}

// WithYearlyDiscount crea un plan con descuento anual específico
func (p PlanMother) WithYearlyDiscount(discountPercent float64) *entity.Plan {
	plan := p.WithDefaults()
	// Calcular precio anual basado en el descuento
	yearlyEquivalent := plan.PriceMonth * 12
	plan.PriceYear = yearlyEquivalent * (1 - discountPercent/100)
	return plan
}

// ExpensivePlan crea un plan costoso para testing de límites
func (p PlanMother) ExpensivePlan() *entity.Plan {
	return p.WithPricing(999.99, 9999.99)
}

// Complete crea un plan con todos los parámetros especificados
func (PlanMother) Complete(id uuid.UUID, name, description string, planType value_object.PlanType,
	status value_object.PlanStatus, maxUsers int, priceMonth, priceYear float64, features []string) *entity.Plan {

	now := time.Now()
	featuresCopy := make([]string, len(features))
	copy(featuresCopy, features)

	return &entity.Plan{
		ID:          id,
		Name:        name,
		Description: description,
		Type:        planType,
		Status:      status,
		MaxUsers:    maxUsers,
		PriceMonth:  priceMonth,
		PriceYear:   priceYear,
		Features:    featuresCopy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Create retorna una nueva instancia de PlanMother
func Create() PlanMother {
	return PlanMother{}
}
