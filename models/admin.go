package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Admin struct {
	ID             primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Email          string             `json:"email" bson:"email"`
	Password       string             `json:"password,omitempty" bson:"password"`
	ProfilePicture string             `json:"profilePicture,omitempty" bson:"profilePicture"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// BranchRequest represents a pending branch creation request
type BranchRequest struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	CompanyID   primitive.ObjectID `json:"companyId" bson:"companyId"`
	BranchData  Branch             `json:"branchData" bson:"branchData"`
	Status      string             `json:"status" bson:"status"` // "pending", "approved", "rejected"
	AdminID     primitive.ObjectID `json:"adminId,omitempty" bson:"adminId,omitempty"`
	AdminNote   string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	SubmittedAt time.Time          `json:"submittedAt" bson:"submittedAt"`
	ProcessedAt time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
}

// AdminDashboardStats represents statistics for the admin dashboard
type AdminDashboardStats struct {
	TotalUsers        int `json:"totalUsers"`
	ActiveUsers       int `json:"activeUsers"`
	TotalCompanies    int `json:"totalCompanies"`
	ActiveCompanies   int `json:"activeCompanies"`
	InactiveCompanies int `json:"inactiveCompanies"`
	TotalBranches     int `json:"totalBranches"`
	PendingBranches   int `json:"pendingBranches"`
	TotalCategories   int `json:"totalCategories"`
}

// CompanyFilter represents filters for company listing
type CompanyFilter struct {
	Status     string    `json:"status,omitempty"`
	Category   string    `json:"category,omitempty"`
	Location   *Location `json:"location,omitempty"`
	StartDate  time.Time `json:"startDate,omitempty"`
	EndDate    time.Time `json:"endDate,omitempty"`
	SearchTerm string    `json:"searchTerm,omitempty"`
}

// BranchApprovalRequest represents the request body for approving/rejecting branches
type BranchApprovalRequest struct {
	Status    string `json:"status"` // "approved" or "rejected"
	AdminNote string `json:"adminNote,omitempty"`
}

// AdminResponse represents the response structure for admin operations
type AdminResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Methods for Admin model
func (a *Admin) ValidateCredentials(email, password string) error {
	// Implementation for validating admin credentials
	return nil
}

func (a *Admin) GetDashboardStats() (*AdminDashboardStats, error) {
	// Implementation for getting dashboard statistics
	return nil, nil
}

func (a *Admin) GetCompanies(filter CompanyFilter) ([]Company, error) {
	// Implementation for getting filtered companies
	return nil, nil
}

func (a *Admin) GetPendingBranchRequests() ([]BranchRequest, error) {
	// Implementation for getting pending branch requests
	return nil, nil
}

func (a *Admin) ProcessBranchRequest(requestID primitive.ObjectID, approval BranchApprovalRequest) error {
	// Implementation for processing branch requests
	return nil
}

func (a *Admin) ManageCategory(category *Category, action string) error {
	// Implementation for CRUD operations on categories
	return nil
}

func (a *Admin) GetCompanyLocations() ([]Location, error) {
	// Implementation for getting company locations
	return nil, nil
}

// AdminWallet represents the actual money movements in the admin wallet
type AdminWallet struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Type        string             `bson:"type" json:"type"` // "subscription_income", "withdrawal_income", "commission_paid"
	Amount      float64            `bson:"amount" json:"amount"`
	Description string             `bson:"description" json:"description"`
	EntityID    primitive.ObjectID `bson:"entityId,omitempty" json:"entityId,omitempty"`     // ID of the related entity (subscription, withdrawal, etc.)
	EntityType  string             `bson:"entityType,omitempty" json:"entityType,omitempty"` // Type of entity
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// AdminWalletBalance represents the current balance of the admin wallet
type AdminWalletBalance struct {
	ID                    primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	TotalIncome           float64            `bson:"totalIncome" json:"totalIncome"`
	TotalWithdrawalIncome float64            `bson:"totalWithdrawalIncome" json:"totalWithdrawalIncome"`
	TotalCommissionsPaid  float64            `bson:"totalCommissionsPaid" json:"totalCommissionsPaid"`
	NetBalance            float64            `bson:"netBalance" json:"netBalance"`
	LastUpdated           time.Time          `bson:"lastUpdated" json:"lastUpdated"`
}
