package model

type Endpoint struct {
	Path     string `gorm:"type:varchar(255);uniqueIndex:idx_path_method" json:"path"`
	Method   string `gorm:"type:varchar(10);uniqueIndex:idx_path_method"  json:"method"`
	Module   string `gorm:"type:varchar(64)"                              json:"module"`
	Kind     string `gorm:"type:varchar(64)"                              json:"kind"`
	Identity string `gorm:"type:varchar(64);primaryKey"                   json:"identity"`
	Remark   string `gorm:"type:varchar(255)"                             json:"remark"`
}

// RoleEndpoint is the join-table row for Role ↔ Endpoint (many2many).
type RoleEndpoint struct {
	RoleID           uint   `gorm:"column:role_id"`
	EndpointIdentity string `gorm:"column:endpoint_identity"`
}

func (RoleEndpoint) TableName() string { return "role_endpoints" }

func (Endpoint) TableName() string { return "endpoints" }
