package model

import "gorm.io/gorm"

type Role struct {
	ID          uint        `gorm:"primaryKey"                                                                                                                           json:"id"`
	Name        string      `gorm:"type:varchar(32);not null;uniqueIndex"                                                                                                json:"name"`
	Description string      `gorm:"type:varchar(255)"                                                                                                                    json:"description"`
	Status      bool        `gorm:"default:true"                                                                                                                         json:"status"`
	Parents     StringSlice `gorm:"type:jsonb;serializer:json"                                                                                                           json:"parents"`
	Endpoints   []*Endpoint `gorm:"many2many:role_endpoints;foreignKey:ID;joinForeignKey:RoleID;References:Identity;joinReferences:EndpointIdentity" json:"endpoints,omitempty"`

	gorm.Model
}

func (Role) TableName() string { return "roles" }

// StringSlice is a helper type for JSON-serialized string arrays in GORM.
type StringSlice []string
