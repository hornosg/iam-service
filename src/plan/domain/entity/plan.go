package entity

import (
	"iam/src/plan/domain/value_object"
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	ID          uuid.UUID
	Name        string
	Description string
	Type        value_object.PlanType
	Status      value_object.PlanStatus
	MaxUsers    int
	PriceMonth  float64
	PriceYear   float64
	Features    []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewPlan(name, description string, planType value_object.PlanType, priceMonth, priceYear float64) *Plan {
	return &Plan{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Type:        planType,
		Status:      value_object.PlanStatusActive,
		MaxUsers:    planType.GetMaxUsers(),
		PriceMonth:  priceMonth,
		PriceYear:   priceYear,
		Features:    []string{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func (p *Plan) UpdateDetails(name, description string) {
	p.Name = name
	p.Description = description
	p.UpdatedAt = time.Now()
}

func (p *Plan) UpdatePricing(priceMonth, priceYear float64) {
	p.PriceMonth = priceMonth
	p.PriceYear = priceYear
	p.UpdatedAt = time.Now()
}

func (p *Plan) ChangeStatus(status value_object.PlanStatus) {
	p.Status = status
	p.UpdatedAt = time.Now()
}

func (p *Plan) AddFeature(feature string) {
	p.Features = append(p.Features, feature)
	p.UpdatedAt = time.Now()
}

func (p *Plan) RemoveFeature(feature string) {
	for i, f := range p.Features {
		if f == feature {
			p.Features = append(p.Features[:i], p.Features[i+1:]...)
			p.UpdatedAt = time.Now()
			break
		}
	}
}

func (p *Plan) HasFeature(feature string) bool {
	for _, f := range p.Features {
		if f == feature {
			return true
		}
	}
	return false
}

func (p *Plan) IsActive() bool {
	return p.Status.IsActive()
}

func (p *Plan) CanBeAssigned() bool {
	return p.Status.CanBeAssigned()
}

func (p *Plan) IsFree() bool {
	return p.Type.IsFree()
}

func (p *Plan) AllowsUsers(count int) bool {
	if p.MaxUsers == -1 { // Unlimited
		return true
	}
	return count <= p.MaxUsers
}

func (p *Plan) GetYearlyDiscount() float64 {
	if p.PriceMonth == 0 {
		return 0
	}
	yearlyEquivalent := p.PriceMonth * 12
	if p.PriceYear >= yearlyEquivalent {
		return 0
	}
	return (yearlyEquivalent - p.PriceYear) / yearlyEquivalent * 100
}
