// models/auth.go

package models

type SignupRequest struct {
	Email           string    `json:"email"`
	Password        string    `json:"password"`
	FullName        string    `json:"fullName"`
	UserType        string    `json:"userType"` // "user", "company", "wholesaler", "serviceProvider"
	DateOfBirth     string    `json:"dateOfBirth"`
	Gender          string    `json:"gender"`
	Phone           string    `json:"phone,omitempty" bson:"phone,omitempty"`
	ProfilePic      string    `json:"profilePic,omitempty" bson:"profilePic,omitempty"`
	ReferralCode    string    `json:"referralCode,omitempty"`
	InterestedDeals []string  `json:"interestedDeals" bson:"interestedDeals"`
	Location        *Location `json:"location" bson:"location"`
	LogoPath        string    `json:"logoPath,omitempty" bson:"logoPath,omitempty"`
	// Only for company signups
	CompanyData *CompanySignupData `json:"companyData,omitempty"`
	// Only for wholesaler signups
	WholesalerData *WholesalerSignupData `json:"wholesalerData,omitempty"`
	// For service provider
	ServiceProviderInfo *ServiceProviderInfo `json:"serviceProviderInfo,omitempty"`
}

type CompanySignupData struct {
	BusinessName string   `json:"businessName"`
	Category     string   `json:"category"`
	SubCategory  string   `json:"subCategory,omitempty"`
	Phones       []string `json:"phones"` // Changed from single phone to array
	Emails       []string `json:"emails"` // Added array of emails
	Address      Address  `json:"address"`
	Logo         string   `json:"logo,omitempty"`         // Base64 encoded or URL
	ReferralCode string   `json:"referralCode,omitempty"` // Referral code field for company signup
}

type WholesalerSignupData struct {
	BusinessName string       `json:"businessName"`
	Category     string       `json:"category"`
	SubCategory  string       `json:"subCategory,omitempty"`
	Phones       []string     `json:"phones"` // Changed from single phone to array
	Emails       []string     `json:"emails"` // Added array of emails
	Address      Address      `json:"address"`
	ReferralCode string       `json:"referralCode,omitempty"`
	SocialMedia  *SocialMedia `json:"socialMedia,omitempty"`
	ContactInfo  *ContactInfo `json:"contactInfo,omitempty"`
}
