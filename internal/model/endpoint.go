package model

type Endpoint struct {
	Path     string `gorm:"type:varchar(255);uniqueIndex:idx_path_method" json:"path"`
	Method   string `gorm:"type:varchar(10);uniqueIndex:idx_path_method"  json:"method"`
	Module   string `gorm:"type:varchar(64)"                              json:"module"`
	Kind     string `gorm:"type:varchar(64)"                              json:"kind"`
	Identity string `gorm:"type:varchar(64);primaryKey"                   json:"identity"`
	Remark   string `gorm:"type:varchar(255)"                             json:"remark"`
}

func (Endpoint) TableName() string { return "endpoints" }
