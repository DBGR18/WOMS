package domain

import "time"

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleSales     Role = "sales"
	RoleScheduler Role = "scheduler"
)

type Priority string

const (
	PriorityLow  Priority = "low"
	PriorityHigh Priority = "high"
)

type OrderStatus string

const (
	StatusPending    OrderStatus = "待排程"
	StatusScheduled  OrderStatus = "已排程"
	StatusInProgress OrderStatus = "生產中"
	StatusCompleted  OrderStatus = "已完成"
	StatusRejected   OrderStatus = "需業務處理"
	StatusCancelled  OrderStatus = "已取消"
)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         Role   `json:"role"`
	LineID       string `json:"lineId,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	Deleted      bool   `json:"deleted,omitempty"`
}

type ProductionLine struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	CapacityPerDay   int    `json:"capacityPerDay"`
	Timezone         string `json:"timezone"`
	ScheduleRevision int64  `json:"scheduleRevision"`
}

type Order struct {
	ID              string      `json:"id"`
	Customer        string      `json:"customer"`
	LineID          string      `json:"lineId"`
	Quantity        int         `json:"quantity"`
	Priority        Priority    `json:"priority"`
	Status          OrderStatus `json:"status"`
	DueDate         time.Time   `json:"dueDate"`
	Note            string      `json:"note,omitempty"`
	CreatedBy       string      `json:"createdBy"`
	SourceOrder     string      `json:"sourceOrder,omitempty"`
	RejectionReason string      `json:"rejectionReason,omitempty"`
	RejectedBy      string      `json:"rejectedBy,omitempty"`
	RejectedAt      time.Time   `json:"rejectedAt,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

type ScheduleAllocation struct {
	OrderID  string      `json:"orderId"`
	LineID   string      `json:"lineId"`
	Date     time.Time   `json:"date"`
	Quantity int         `json:"quantity"`
	Priority Priority    `json:"priority"`
	Locked   bool        `json:"locked"`
	Status   OrderStatus `json:"status,omitempty"`
}

type ScheduleConflict struct {
	OrderID            string    `json:"orderId"`
	Reason             string    `json:"reason"`
	EarliestFinishDate time.Time `json:"earliestFinishDate"`
	AffectedOrderIDs   []string  `json:"affectedOrderIds,omitempty"`
}

type ScheduleJobStatus string

const (
	JobQueued    ScheduleJobStatus = "queued"
	JobRunning   ScheduleJobStatus = "running"
	JobCompleted ScheduleJobStatus = "completed"
	JobFailed    ScheduleJobStatus = "failed"
	JobCancelled ScheduleJobStatus = "cancelled"
)

type ScheduleJob struct {
	ID           string            `json:"id"`
	LineID       string            `json:"lineId"`
	Status       ScheduleJobStatus `json:"status"`
	Message      string            `json:"message,omitempty"`
	Source       string            `json:"source,omitempty"`
	PreviewID    string            `json:"previewId,omitempty"`
	RequestHash  string            `json:"requestHash,omitempty"`
	LineRevision int64             `json:"lineRevision,omitempty"`
	AttemptCount int               `json:"attemptCount,omitempty"`
	OrderIDs     []string          `json:"orderIds,omitempty"`
	StartedAt    time.Time         `json:"startedAt,omitempty"`
	CompletedAt  time.Time         `json:"completedAt,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type AuditEntry struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actorId"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
