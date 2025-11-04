package models

import (
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Duration constants for common sponsorship periods
const (
	MinDurationDays = 1      // Minimum duration in days
	MaxDurationDays = 365    // Maximum duration in days (1 year)
	DefaultDuration = 30     // Default duration in days
	
	// Common duration presets
	DurationWeek     = 7     // 1 week
	DurationMonth    = 30    // 1 month
	DurationQuarter  = 90    // 3 months
	DurationHalfYear = 180   // 6 months
	DurationYear     = 365   // 1 year
)

// Duration validation errors
var (
	ErrDurationTooShort = errors.New("duration must be at least 1 day")
	ErrDurationTooLong  = errors.New("duration cannot exceed 365 days")
	ErrInvalidDuration  = errors.New("duration must be a positive integer")
)

// DurationUnit represents the unit of duration
type DurationUnit string

const (
	DurationUnitDays   DurationUnit = "days"
	DurationUnitWeeks  DurationUnit = "weeks"
	DurationUnitMonths DurationUnit = "months"
	DurationUnitYears  DurationUnit = "years"
)

// DurationInfo provides additional information about the duration
type DurationInfo struct {
	Days    int         `json:"days" bson:"days"`
	Weeks   int         `json:"weeks" bson:"weeks"`
	Months  int         `json:"months" bson:"months"`
	Years   int         `json:"years" bson:"years"`
	Unit    DurationUnit `json:"unit" bson:"unit"`
	IsValid bool        `json:"isValid" bson:"isValid"`
}

// Sponsorship represents a sponsorship card that can be created by admins
type Sponsorship struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Title       string             `json:"title" bson:"title" validate:"required"`
	Price       float64            `json:"price" bson:"price" validate:"required,gt=0"`
	Duration    int                `json:"duration" bson:"duration" validate:"required,min=1,max=365"` // Duration in days
	DurationInfo DurationInfo      `json:"durationInfo,omitempty" bson:"durationInfo,omitempty"`       // Calculated duration breakdown
	Discount    float64            `json:"discount" bson:"discount" validate:"gte=0,lte=100"` // Discount percentage
	UsedCount   int                `json:"usedCount" bson:"usedCount"`                        // Current usage count
	StartDate   time.Time          `json:"startDate" bson:"startDate" validate:"required"`
	EndDate     time.Time          `json:"endDate" bson:"endDate" validate:"required"`
	CreatedBy   primitive.ObjectID `json:"createdBy" bson:"createdBy"` // Admin who created this sponsorship
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// SponsorshipRequest represents the request body for creating/updating sponsorships
type SponsorshipRequest struct {
	Title     string    `json:"title" validate:"required"`
	Price     float64   `json:"price" validate:"required,gt=0"`
	Duration  int       `json:"duration" validate:"required,min=1,max=365"`
	Discount  float64   `json:"discount" validate:"gte=0,lte=100"`
	StartDate time.Time `json:"startDate" validate:"required"`
	EndDate   time.Time `json:"endDate" validate:"required"`
}

// SponsorshipUpdateRequest represents the request body for updating sponsorships
type SponsorshipUpdateRequest struct {
	Title     *string    `json:"title,omitempty"`
	Price     *float64   `json:"price,omitempty" validate:"omitempty,gt=0"`
	Duration  *int       `json:"duration,omitempty" validate:"omitempty,min=1,max=365"`
	Discount  *float64   `json:"discount,omitempty" validate:"omitempty,gte=0,lte=100"`
	StartDate *time.Time `json:"startDate,omitempty"`
	EndDate   *time.Time `json:"endDate,omitempty"`
}

// ValidateDuration validates the duration field with enhanced checks
func (sr *SponsorshipRequest) ValidateDuration() error {
	if sr.Duration < MinDurationDays {
		return ErrDurationTooShort
	}
	if sr.Duration > MaxDurationDays {
		return ErrDurationTooLong
	}
	return nil
}

// ValidateDurationUpdate validates duration updates with enhanced checks
func (sur *SponsorshipUpdateRequest) ValidateDurationUpdate() error {
	if sur.Duration != nil {
		if *sur.Duration < MinDurationDays {
			return ErrDurationTooShort
		}
		if *sur.Duration > MaxDurationDays {
			return ErrDurationTooLong
		}
	}
	return nil
}

// CalculateDurationInfo calculates and returns detailed duration information
func CalculateDurationInfo(days int) DurationInfo {
	if days < MinDurationDays || days > MaxDurationDays {
		return DurationInfo{IsValid: false}
	}

	info := DurationInfo{
		Days:    days,
		Weeks:   days / 7,
		Months:  days / 30,
		Years:   days / 365,
		Unit:    DurationUnitDays,
		IsValid: true,
	}

	// Determine the most appropriate unit for display
	if days >= 365 {
		info.Unit = DurationUnitYears
	} else if days >= 30 {
		info.Unit = DurationUnitMonths
	} else if days >= 7 {
		info.Unit = DurationUnitWeeks
	}

	return info
}

// GetDurationDescription returns a human-readable description of the duration
func (di DurationInfo) GetDurationDescription() string {
	if !di.IsValid {
		return "Invalid duration"
	}

	switch di.Unit {
	case DurationUnitYears:
		if di.Years == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", di.Years)
	case DurationUnitMonths:
		if di.Months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", di.Months)
	case DurationUnitWeeks:
		if di.Weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", di.Weeks)
	default:
		if di.Days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", di.Days)
	}
}

// IsDurationValid checks if the duration is within acceptable limits
func IsDurationValid(days int) bool {
	return days >= MinDurationDays && days <= MaxDurationDays
}

// GetRecommendedDurations returns a list of recommended duration options
func GetRecommendedDurations() []int {
	return []int{
		DurationWeek,     // 7 days
		DurationMonth,    // 30 days
		DurationQuarter,  // 90 days
		DurationHalfYear, // 180 days
		DurationYear,     // 365 days
	}
}
